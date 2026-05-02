// Package postgres implements processing persistence.
package postgres

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"

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

func nullableString(value string) any {
	if value == "" {
		return nil
	}
	return value
}
