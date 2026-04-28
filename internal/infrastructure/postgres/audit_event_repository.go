// Package postgres provides PostgreSQL repository implementations.
package postgres

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"

	"audit-go/internal/domain"
)

type AuditEventRepository struct {
	db *sql.DB
}

// NewAuditEventRepository creates a PostgreSQL audit event repository.
func NewAuditEventRepository(db *sql.DB) *AuditEventRepository {
	return &AuditEventRepository{db: db}
}

// Save persists an audit event.
func (r *AuditEventRepository) Save(
	ctx context.Context,
	event domain.AuditEvent,
) error {
	metadata, err := json.Marshal(event.Metadata)
	if err != nil {
		return fmt.Errorf("marshaling metadata: %w", err)
	}

	query := `
		INSERT INTO audit_events (
			id,
			tenant_id,
			actor_id,
			action,
			target_id,
			target_type,
			occurred_at,
			request_id,
			metadata
		)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9)
	`

	_, err = r.db.ExecContext(
		ctx,
		query,
		event.ID,
		event.TenantID,
		event.ActorID,
		string(event.Action),
		event.TargetID,
		string(event.TargetType),
		event.OccurredAt,
		event.RequestID,
		metadata,
	)
	if err != nil {
		return fmt.Errorf("saving audit event: %w", err)
	}

	return nil
}

// FindByTarget returns events by target id.
func (r *AuditEventRepository) FindByTarget(
	ctx context.Context,
	targetID string,
) ([]domain.AuditEvent, error) {
	query := `
		SELECT
			id,
			tenant_id,
			actor_id,
			action,
			target_id,
			target_type,
			occurred_at,
			request_id,
			metadata
		FROM audit_events
		WHERE target_id = $1
		ORDER BY occurred_at DESC
	`

	rows, err := r.db.QueryContext(ctx, query, targetID)
	if err != nil {
		return nil, fmt.Errorf("querying audit events by target: %w", err)
	}
	defer func() { _ = rows.Close() }()

	return scanAuditEvents(rows)
}

// FindByTenant returns events by tenant id.
func (r *AuditEventRepository) FindByTenant(
	ctx context.Context,
	tenantID string,
) ([]domain.AuditEvent, error) {
	query := `
		SELECT
			id,
			tenant_id,
			actor_id,
			action,
			target_id,
			target_type,
			occurred_at,
			request_id,
			metadata
		FROM audit_events
		WHERE tenant_id = $1
		ORDER BY occurred_at DESC
	`

	rows, err := r.db.QueryContext(ctx, query, tenantID)
	if err != nil {
		return nil, fmt.Errorf("querying audit events by tenant: %w", err)
	}
	defer func() { _ = rows.Close() }()

	return scanAuditEvents(rows)
}

func scanAuditEvents(rows *sql.Rows) ([]domain.AuditEvent, error) {
	var events []domain.AuditEvent

	for rows.Next() {
		var event domain.AuditEvent
		var action string
		var targetType string
		var metadata []byte

		if err := rows.Scan(
			&event.ID,
			&event.TenantID,
			&event.ActorID,
			&action,
			&event.TargetID,
			&targetType,
			&event.OccurredAt,
			&event.RequestID,
			&metadata,
		); err != nil {
			return nil, fmt.Errorf("scanning audit event row: %w", err)
		}

		event.Action = domain.Action(action)
		event.TargetType = domain.TargetType(targetType)
		event.OccurredAt = event.OccurredAt.UTC()

		if len(metadata) > 0 {
			if err := json.Unmarshal(metadata, &event.Metadata); err != nil {
				return nil, fmt.Errorf("unmarshaling metadata: %w", err)
			}
		}

		events = append(events, event)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterating audit event rows: %w", err)
	}

	return events, nil
}
