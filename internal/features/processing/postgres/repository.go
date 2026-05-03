// Package postgres implements processing persistence.
package postgres

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"

	"audit-go/internal/features/audit"
	"audit-go/internal/features/documents"
	"audit-go/internal/features/processing"
	platformpostgres "audit-go/internal/platform/postgres"
)

// Repository stores outbox events and processing jobs in PostgreSQL.
type Repository struct {
	db *sql.DB
}

// NewRepository creates a PostgreSQL-backed processing repository.
func NewRepository(db *sql.DB) *Repository {
	return &Repository{db: db}
}

// ClaimNextJob atomically locks one available parse job for a worker.
func (r *Repository) ClaimNextJob(ctx context.Context, workerID string, lockDuration time.Duration) (*processing.Job, error) {
	if lockDuration <= 0 {
		lockDuration = 5 * time.Minute
	}
	lockedUntil := time.Now().UTC().Add(lockDuration)

	tx, err := r.db.BeginTx(ctx, &sql.TxOptions{Isolation: sql.LevelReadCommitted})
	if err != nil {
		return nil, fmt.Errorf("beginning claim transaction: %w", err)
	}

	const query = `
		WITH next_job AS (
			SELECT id
			FROM processing_jobs
			WHERE
				job_type = $1
				AND (
					(status IN ($2, $3) AND available_at <= NOW())
					OR (status = $4 AND (locked_until IS NULL OR locked_until < NOW()))
				)
			ORDER BY available_at ASC, created_at ASC
			FOR UPDATE SKIP LOCKED
			LIMIT 1
		),
		claimed AS (
			UPDATE processing_jobs pj
			SET
				status = $4,
				locked_by = $5,
				locked_until = $6,
				attempts = attempts + 1,
				last_error = NULL,
				updated_at = NOW()
			FROM next_job
			WHERE pj.id = next_job.id
			RETURNING
				pj.id,
				pj.job_type,
				pj.aggregate_type,
				pj.aggregate_id,
				pj.status,
				pj.payload,
				pj.idempotency_key,
				pj.attempts,
				pj.max_attempts,
				pj.available_at,
				pj.locked_by,
				pj.locked_until,
				pj.last_error,
				pj.created_at,
				pj.updated_at
		)
		SELECT * FROM claimed
	`

	job, err := scanJob(tx.QueryRowContext(
		ctx,
		query,
		processing.JobTypeParseDocument,
		string(processing.JobQueued),
		string(processing.JobRetryScheduled),
		string(processing.JobRunning),
		workerID,
		lockedUntil,
	))
	if errors.Is(err, sql.ErrNoRows) {
		_ = tx.Rollback()
		return nil, nil
	}
	if err != nil {
		_ = tx.Rollback()
		return nil, fmt.Errorf("claiming processing job: %w", err)
	}

	res, err := tx.ExecContext(
		ctx,
		`UPDATE documents
		 SET status = $1,
			 processed = FALSE
		 WHERE id = $2
		   AND status <> $3`,
		string(documents.StatusProcessing),
		job.AggregateID,
		string(documents.StatusDeleted),
	)
	if err != nil {
		_ = tx.Rollback()
		return nil, fmt.Errorf("marking document as processing: %w", err)
	}
	if rows, rowsErr := res.RowsAffected(); rowsErr != nil {
		_ = tx.Rollback()
		return nil, fmt.Errorf("checking processing document update: %w", rowsErr)
	} else if rows == 0 {
		_ = tx.Rollback()
		return nil, fmt.Errorf("marking document as processing: document %s not found or deleted", job.AggregateID)
	}

	if err = tx.Commit(); err != nil {
		return nil, fmt.Errorf("committing claim transaction: %w", err)
	}

	return job, nil
}

// SaveOutboxEvent persists an event to publish after transaction commit.
func (r *Repository) SaveOutboxEvent(ctx context.Context, event processing.OutboxEvent) error {
	payload, err := json.Marshal(event.Payload)
	if err != nil {
		return fmt.Errorf("marshaling outbox payload: %w", err)
	}

	const query = `
		INSERT INTO outbox_events (
			id,
			event_type,
			aggregate_type,
			aggregate_id,
			payload,
			status,
			attempts,
			last_error,
			created_at,
			published_at
		)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10)
	`

	_, err = platformpostgres.Executor(ctx, r.db).ExecContext(
		ctx,
		query,
		event.ID,
		event.EventType,
		event.AggregateType,
		event.AggregateID,
		payload,
		string(event.Status),
		event.Attempts,
		nullableString(event.LastError),
		event.CreatedAt,
		event.PublishedAt,
	)
	if err != nil {
		return fmt.Errorf("saving outbox event: %w", err)
	}

	return nil
}

// SaveJob persists a processing job. The idempotency key prevents duplicates.
func (r *Repository) SaveJob(ctx context.Context, job processing.Job) error {
	payload, err := json.Marshal(job.Payload)
	if err != nil {
		return fmt.Errorf("marshaling job payload: %w", err)
	}

	const query = `
		INSERT INTO processing_jobs (
			id,
			job_type,
			aggregate_type,
			aggregate_id,
			status,
			payload,
			idempotency_key,
			attempts,
			max_attempts,
			available_at,
			locked_by,
			locked_until,
			last_error,
			created_at,
			updated_at
		)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15)
		ON CONFLICT (idempotency_key) DO NOTHING
	`

	_, err = platformpostgres.Executor(ctx, r.db).ExecContext(
		ctx,
		query,
		job.ID,
		job.JobType,
		job.AggregateType,
		job.AggregateID,
		string(job.Status),
		payload,
		job.IdempotencyKey,
		job.Attempts,
		job.MaxAttempts,
		job.AvailableAt,
		nullableString(job.LockedBy),
		job.LockedUntil,
		nullableString(job.LastError),
		job.CreatedAt,
		job.UpdatedAt,
	)
	if err != nil {
		return fmt.Errorf("saving processing job: %w", err)
	}

	return nil
}

// CompleteParseJob persists parse artifacts and marks the job/document completed.
func (r *Repository) CompleteParseJob(ctx context.Context, jobID string, result processing.ParseResult) error {
	if result.DocumentID == "" {
		return errors.New("complete parse job: document id is required")
	}
	if result.ParsedAt.IsZero() {
		result.ParsedAt = time.Now().UTC()
	}

	tables, err := json.Marshal(result.Tables)
	if err != nil {
		return fmt.Errorf("marshaling parsed tables: %w", err)
	}

	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("beginning complete transaction: %w", err)
	}

	const saveResult = `
		INSERT INTO document_parse_results (
			document_id,
			filename,
			pages,
			text,
			markdown,
			tables,
			created_at,
			updated_at
		)
		VALUES ($1,$2,$3,$4,$5,$6::jsonb,$7,$7)
		ON CONFLICT (document_id) DO UPDATE SET
			filename = EXCLUDED.filename,
			pages = EXCLUDED.pages,
			text = EXCLUDED.text,
			markdown = EXCLUDED.markdown,
			tables = EXCLUDED.tables,
			updated_at = EXCLUDED.updated_at
	`
	if _, err = tx.ExecContext(
		ctx,
		saveResult,
		result.DocumentID,
		result.Filename,
		result.Pages,
		result.Text,
		result.Markdown,
		tables,
		result.ParsedAt,
	); err != nil {
		_ = tx.Rollback()
		return fmt.Errorf("saving parse result: %w", err)
	}

	if _, err = tx.ExecContext(ctx, `DELETE FROM document_chunks WHERE document_id = $1`, result.DocumentID); err != nil {
		_ = tx.Rollback()
		return fmt.Errorf("clearing old document chunks: %w", err)
	}

	for i, chunk := range result.Chunks {
		index := chunk.Index
		if index < 0 || (index == 0 && i > 0) {
			index = i
		}
		if strings.TrimSpace(chunk.Content) == "" {
			continue
		}
		if err = insertChunk(ctx, tx, result.DocumentID, index, chunk); err != nil {
			_ = tx.Rollback()
			return err
		}
	}

	res, err := tx.ExecContext(
		ctx,
		`UPDATE documents
		 SET status = $1,
			 processed = TRUE
		 WHERE id = $2
		   AND status <> $3`,
		string(documents.StatusParsed),
		result.DocumentID,
		string(documents.StatusDeleted),
	)
	if err != nil {
		_ = tx.Rollback()
		return fmt.Errorf("marking document parsed: %w", err)
	}
	if rows, rowsErr := res.RowsAffected(); rowsErr != nil {
		_ = tx.Rollback()
		return fmt.Errorf("checking parsed document update: %w", rowsErr)
	} else if rows == 0 {
		_ = tx.Rollback()
		return fmt.Errorf("marking document parsed: document %s not found or deleted", result.DocumentID)
	}

	res, err = tx.ExecContext(
		ctx,
		`UPDATE processing_jobs
		 SET status = $1,
			 locked_by = NULL,
			 locked_until = NULL,
			 last_error = NULL,
			 updated_at = NOW()
		 WHERE id = $2`,
		string(processing.JobCompleted),
		jobID,
	)
	if err != nil {
		_ = tx.Rollback()
		return fmt.Errorf("marking job completed: %w", err)
	}
	if rows, rowsErr := res.RowsAffected(); rowsErr != nil {
		_ = tx.Rollback()
		return fmt.Errorf("checking completed job update: %w", rowsErr)
	} else if rows == 0 {
		_ = tx.Rollback()
		return fmt.Errorf("marking job completed: job %s not found", jobID)
	}

	if err = insertParsedAuditEvent(ctx, tx, jobID, result); err != nil {
		_ = tx.Rollback()
		return err
	}

	if err = tx.Commit(); err != nil {
		return fmt.Errorf("committing complete transaction: %w", err)
	}

	return nil
}

// RecordJobFailure schedules a retry or dead-letters the job after a processing failure.
func (r *Repository) RecordJobFailure(ctx context.Context, job processing.Job, failure error, retryDelay time.Duration) error {
	if retryDelay < 0 {
		retryDelay = 0
	}
	lastError := truncateError(failure)
	status := processing.JobRetryScheduled
	documentStatus := documents.StatusQueued
	availableAt := time.Now().UTC().Add(retryDelay)

	if job.Attempts >= job.MaxAttempts {
		status = processing.JobDeadLetter
		documentStatus = documents.StatusFailed
		availableAt = time.Now().UTC()
	}

	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("beginning failure transaction: %w", err)
	}

	res, err := tx.ExecContext(
		ctx,
		`UPDATE processing_jobs
		 SET status = $1,
			 available_at = $2,
			 locked_by = NULL,
			 locked_until = NULL,
			 last_error = $3,
			 updated_at = NOW()
		 WHERE id = $4`,
		string(status),
		availableAt,
		lastError,
		job.ID,
	)
	if err != nil {
		_ = tx.Rollback()
		return fmt.Errorf("recording job failure: %w", err)
	}
	if rows, rowsErr := res.RowsAffected(); rowsErr != nil {
		_ = tx.Rollback()
		return fmt.Errorf("checking failed job update: %w", rowsErr)
	} else if rows == 0 {
		_ = tx.Rollback()
		return fmt.Errorf("recording job failure: job %s not found", job.ID)
	}

	if _, err = tx.ExecContext(
		ctx,
		`UPDATE documents
		 SET status = $1,
			 processed = FALSE
		 WHERE id = $2
		   AND status <> $3`,
		string(documentStatus),
		job.AggregateID,
		string(documents.StatusDeleted),
	); err != nil {
		_ = tx.Rollback()
		return fmt.Errorf("marking document after job failure: %w", err)
	}

	if err = tx.Commit(); err != nil {
		return fmt.Errorf("committing failure transaction: %w", err)
	}

	return nil
}

// FindLatestJobByAggregate returns the latest processing job for an aggregate.
func (r *Repository) FindLatestJobByAggregate(
	ctx context.Context,
	aggregateType string,
	aggregateID string,
) (*processing.Job, error) {
	const query = `
		SELECT
			id,
			job_type,
			aggregate_type,
			aggregate_id,
			status,
			payload,
			idempotency_key,
			attempts,
			max_attempts,
			available_at,
			locked_by,
			locked_until,
			last_error,
			created_at,
			updated_at
		FROM processing_jobs
		WHERE aggregate_type = $1
		  AND aggregate_id = $2
		ORDER BY updated_at DESC, created_at DESC
		LIMIT 1
	`

	job, err := scanJob(platformpostgres.Executor(ctx, r.db).QueryRowContext(ctx, query, aggregateType, aggregateID))
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("finding latest processing job: %w", err)
	}

	return job, nil
}

// FindParseResultSummary returns a lightweight view of persisted parse artifacts.
func (r *Repository) FindParseResultSummary(ctx context.Context, documentID string) (*processing.ParseResultSummary, error) {
	const query = `
		SELECT
			r.document_id,
			r.filename,
			COALESCE(r.pages, 0),
			octet_length(r.text),
			octet_length(r.markdown),
			jsonb_array_length(r.tables),
			r.updated_at,
			COUNT(c.id)
		FROM document_parse_results r
		LEFT JOIN document_chunks c ON c.document_id = r.document_id
		WHERE r.document_id = $1
		GROUP BY
			r.document_id,
			r.filename,
			r.pages,
			r.text,
			r.markdown,
			r.tables,
			r.updated_at
	`

	var summary processing.ParseResultSummary
	var parsedAt time.Time
	if err := platformpostgres.Executor(ctx, r.db).QueryRowContext(ctx, query, documentID).Scan(
		&summary.DocumentID,
		&summary.Filename,
		&summary.Pages,
		&summary.TextBytes,
		&summary.MarkdownBytes,
		&summary.TablesCount,
		&parsedAt,
		&summary.ChunksCount,
	); errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	} else if err != nil {
		return nil, fmt.Errorf("finding parse result summary: %w", err)
	}

	parsedAt = parsedAt.UTC()
	summary.LastParsedAt = &parsedAt

	return &summary, nil
}

func nullableString(value string) any {
	if value == "" {
		return nil
	}
	return value
}

type jobScanner interface {
	Scan(dest ...any) error
}

func scanJob(scanner jobScanner) (*processing.Job, error) {
	var job processing.Job
	var payload []byte
	var status string
	var lockedBy sql.NullString
	var lockedUntil sql.NullTime
	var lastError sql.NullString

	err := scanner.Scan(
		&job.ID,
		&job.JobType,
		&job.AggregateType,
		&job.AggregateID,
		&status,
		&payload,
		&job.IdempotencyKey,
		&job.Attempts,
		&job.MaxAttempts,
		&job.AvailableAt,
		&lockedBy,
		&lockedUntil,
		&lastError,
		&job.CreatedAt,
		&job.UpdatedAt,
	)
	if err != nil {
		return nil, err
	}

	if len(payload) > 0 {
		if err = json.Unmarshal(payload, &job.Payload); err != nil {
			return nil, fmt.Errorf("unmarshaling job payload: %w", err)
		}
	}
	if job.Payload == nil {
		job.Payload = map[string]any{}
	}

	job.Status = processing.JobStatus(status)
	if lockedBy.Valid {
		job.LockedBy = lockedBy.String
	}
	if lockedUntil.Valid {
		value := lockedUntil.Time.UTC()
		job.LockedUntil = &value
	}
	if lastError.Valid {
		job.LastError = lastError.String
	}
	job.AvailableAt = job.AvailableAt.UTC()
	job.CreatedAt = job.CreatedAt.UTC()
	job.UpdatedAt = job.UpdatedAt.UTC()

	return &job, nil
}

func insertChunk(ctx context.Context, tx *sql.Tx, documentID string, index int, chunk processing.DocumentChunk) error {
	if len(chunk.Embedding) == 0 {
		_, err := tx.ExecContext(
			ctx,
			`INSERT INTO document_chunks (document_id, chunk_index, content, embedding)
			 VALUES ($1,$2,$3,NULL)`,
			documentID,
			index,
			chunk.Content,
		)
		if err != nil {
			return fmt.Errorf("inserting document chunk: %w", err)
		}
		return nil
	}

	vector, err := formatVector(chunk.Embedding)
	if err != nil {
		return err
	}

	_, err = tx.ExecContext(
		ctx,
		`INSERT INTO document_chunks (document_id, chunk_index, content, embedding)
		 VALUES ($1,$2,$3,$4::vector)`,
		documentID,
		index,
		chunk.Content,
		vector,
	)
	if err != nil {
		return fmt.Errorf("inserting embedded document chunk: %w", err)
	}

	return nil
}

func insertParsedAuditEvent(ctx context.Context, tx *sql.Tx, jobID string, result processing.ParseResult) error {
	metadata, err := json.Marshal(map[string]string{
		"filename": result.Filename,
		"pages":    strconv.Itoa(result.Pages),
		"tables":   strconv.Itoa(len(result.Tables)),
		"chunks":   strconv.Itoa(len(result.Chunks)),
	})
	if err != nil {
		return fmt.Errorf("marshaling parsed audit metadata: %w", err)
	}

	res, err := tx.ExecContext(
		ctx,
		`INSERT INTO audit_events (
			id,
			actor_id,
			action,
			target_id,
			target_type,
			occurred_at,
			request_id,
			metadata
		)
		SELECT
			$1,
			uploaded_by,
			$2,
			id,
			$3,
			$4,
			$5,
			$6::jsonb
		FROM documents
		WHERE id = $7`,
		uuid.NewString(),
		string(audit.ActionDocumentParsed),
		string(audit.TargetDocument),
		result.ParsedAt,
		jobID,
		metadata,
		result.DocumentID,
	)
	if err != nil {
		return fmt.Errorf("saving parsed audit event: %w", err)
	}
	if rows, rowsErr := res.RowsAffected(); rowsErr != nil {
		return fmt.Errorf("checking parsed audit insert: %w", rowsErr)
	} else if rows == 0 {
		return fmt.Errorf("saving parsed audit event: document %s not found", result.DocumentID)
	}

	return nil
}

func formatVector(values []float64) (string, error) {
	if len(values) != 1536 {
		return "", fmt.Errorf("embedding vector has %d dimensions, expected 1536", len(values))
	}

	var builder strings.Builder
	builder.WriteByte('[')
	for i, value := range values {
		if i > 0 {
			builder.WriteByte(',')
		}
		builder.WriteString(strconv.FormatFloat(value, 'f', -1, 64))
	}
	builder.WriteByte(']')

	return builder.String(), nil
}

func truncateError(err error) string {
	if err == nil {
		return ""
	}
	message := err.Error()
	if len(message) <= 4000 {
		return message
	}
	return message[:4000]
}
