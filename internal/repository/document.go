// Package repository defines persistence contracts.
package repository

import (
	"context"

	"audit-go/internal/domain"
)

// DocumentRepository defines document storage operations.
type DocumentRepository interface {
	// Save inserts or updates a document.
	Save(ctx context.Context, doc domain.Document) error

	// FindByID returns a document by id.
	FindByID(ctx context.Context, id string) (*domain.Document, error)

	// FindByJVID returns documents by joint venture id.
	FindByJVID(ctx context.Context, jvID string) ([]domain.Document, error)

	// FindUnprocessed returns pending documents.
	FindUnprocessed(ctx context.Context) ([]domain.Document, error)

	// MarkProcessed marks a document as processed.
	MarkProcessed(ctx context.Context, id string) error

	// Delete removes a document by id.
	Delete(ctx context.Context, id string) error
}
