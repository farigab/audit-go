// Package repository defines persistence contracts.
package repository

import (
	"context"

	"audit-go/internal/domain"
)

// AuditEventRepository defines audit event storage operations.
type AuditEventRepository interface {
	// Save persists an audit event.
	Save(ctx context.Context, event domain.AuditEvent) error

	// FindByTarget returns events by target id.
	FindByTarget(ctx context.Context, targetID string) ([]domain.AuditEvent, error)

}
