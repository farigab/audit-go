// Package postgres provides PostgreSQL repository implementations.
package postgres

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"audit-go/internal/domain"
)

type DocumentRepository struct {
	db *sql.DB
}

// NewDocumentRepository creates a new PostgreSQL document repository.
func NewDocumentRepository(db *sql.DB) *DocumentRepository {
	return &DocumentRepository{db: db}
}

// Save inserts or updates a document.
func (r *DocumentRepository) Save(ctx context.Context, doc domain.Document) error {
	query := `
		INSERT INTO documents (
			id,
			jv_id,
			name,
			type,
			storage_key,
			uploaded_by,
			uploaded_at,
			processed
		)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9)
		ON CONFLICT (id) DO UPDATE SET
			name        = EXCLUDED.name,
			type        = EXCLUDED.type,
			storage_key = EXCLUDED.storage_key,
			processed   = EXCLUDED.processed
	`

	_, err := r.db.ExecContext(
		ctx,
		query,
		doc.ID,
		doc.JVID,
		doc.Name,
		string(doc.Type),
		doc.StorageKey,
		doc.UploadedBy,
		doc.UploadedAt,
		doc.Processed,
	)
	if err != nil {
		return fmt.Errorf("saving document: %w", err)
	}

	return nil
}

// FindByID returns a document by id.
func (r *DocumentRepository) FindByID(
	ctx context.Context,
	id string,
) (*domain.Document, error) {
	query := `
		SELECT
			id,
			jv_id,
			name,
			type,
			storage_key,
			uploaded_by,
			uploaded_at,
			processed
		FROM documents
		WHERE id = $1
	`

	row := r.db.QueryRowContext(ctx, query, id)

	return scanDocument(row)
}

// FindByJVID returns documents by joint venture id.
func (r *DocumentRepository) FindByJVID(
	ctx context.Context,
	jvID string,
) ([]domain.Document, error) {
	query := `
		SELECT
			id,
			jv_id,
			name,
			type,
			storage_key,
			uploaded_by,
			uploaded_at,
			processed
		FROM documents
		WHERE jv_id = $1
		ORDER BY uploaded_at DESC
	`

	rows, err := r.db.QueryContext(ctx, query, jvID)
	if err != nil {
		return nil, fmt.Errorf("querying documents by jv_id: %w", err)
	}
	defer func() { _ = rows.Close() }()

	return scanDocuments(rows)
}

// FindUnprocessed returns pending documents.
func (r *DocumentRepository) FindUnprocessed(
	ctx context.Context,
) ([]domain.Document, error) {
	query := `
		SELECT
			id,
			jv_id,
			name,
			type,
			storage_key,
			uploaded_by,
			uploaded_at,
			processed
		FROM documents
		WHERE processed = FALSE
		ORDER BY uploaded_at ASC
		LIMIT 10
	`

	rows, err := r.db.QueryContext(ctx, query)
	if err != nil {
		return nil, fmt.Errorf("querying unprocessed documents: %w", err)
	}
	defer func() { _ = rows.Close() }()

	return scanDocuments(rows)
}

// MarkProcessed marks a document as processed.
func (r *DocumentRepository) MarkProcessed(
	ctx context.Context,
	id string,
) error {
	query := `
		UPDATE documents
		SET processed = TRUE
		WHERE id = $1
	`

	_, err := r.db.ExecContext(ctx, query, id)
	if err != nil {
		return fmt.Errorf("marking document as processed: %w", err)
	}

	return nil
}

// Delete removes a document by id.
func (r *DocumentRepository) Delete(
	ctx context.Context,
	id string,
) error {
	query := `DELETE FROM documents WHERE id = $1`

	res, err := r.db.ExecContext(ctx, query, id)
	if err != nil {
		return fmt.Errorf("deleting document: %w", err)
	}

	rows, err := res.RowsAffected()
	if err != nil {
		return fmt.Errorf("checking rows affected: %w", err)
	}

	if rows == 0 {
		return errors.New("document not found")
	}

	return nil
}

func scanDocument(row *sql.Row) (*domain.Document, error) {
	var doc domain.Document
	var docType string
	var uploadedAt time.Time

	err := row.Scan(
		&doc.ID,
		&doc.JVID,
		&doc.Name,
		&docType,
		&doc.StorageKey,
		&doc.UploadedBy,
		&uploadedAt,
		&doc.Processed,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, errors.New("document not found")
	}
	if err != nil {
		return nil, fmt.Errorf("scanning document: %w", err)
	}

	doc.Type = domain.DocType(docType)
	doc.UploadedAt = uploadedAt.UTC()

	return &doc, nil
}

func scanDocuments(rows *sql.Rows) ([]domain.Document, error) {
	var docs []domain.Document

	for rows.Next() {
		var doc domain.Document
		var docType string
		var uploadedAt time.Time

		if err := rows.Scan(
			&doc.ID,
			&doc.JVID,
			&doc.Name,
			&docType,
			&doc.StorageKey,
			&doc.UploadedBy,
			&uploadedAt,
			&doc.Processed,
		); err != nil {
			return nil, fmt.Errorf("scanning document row: %w", err)
		}

		doc.Type = domain.DocType(docType)
		doc.UploadedAt = uploadedAt.UTC()

		docs = append(docs, doc)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterating document rows: %w", err)
	}

	return docs, nil
}
