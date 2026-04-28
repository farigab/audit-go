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

func NewAuditEventRepository(db *sql.DB) *AuditEventRepository {
	return &AuditEventRepository{db: db}
}

func (r *AuditEventRepository) Save(event domain.AuditEvent) error {
	metadata, err := json.Marshal(event.Metadata)
	if err != nil {
		return fmt.Errorf("marshaling metadata: %w", err)
	}

	query := `
		INSERT INTO audit_events
			(id, tenant_id, actor_id, action, target_id, target_type, occurred_at, request_id, metadata)
		VALUES
			($1, $2, $3, $4, $5, $6, $7, $8, $9)
	`

	_, err = r.db.ExecContext(
		context.Background(),
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
		return fmt.Errorf("inserting audit event: %w", err)
	}

	return nil
}

func (r *AuditEventRepository) FindByTarget(targetID string) ([]domain.AuditEvent, error) {
	query := `
		SELECT id, tenant_id, actor_id, action, target_id, target_type, occurred_at, request_id, metadata
		FROM audit_events
		WHERE target_id = $1
		ORDER BY occurred_at DESC
	`

	rows, err := r.db.QueryContext(context.Background(), query, targetID)
	if err != nil {
		return nil, fmt.Errorf("querying audit events by target: %w", err)
	}
	defer func() { _ = rows.Close() }()

	return scanAuditEvents(rows)
}

func (r *AuditEventRepository) FindByTenant(tenantID string) ([]domain.AuditEvent, error) {
	query := `
		SELECT id, tenant_id, actor_id, action, target_id, target_type, occurred_at, request_id, metadata
		FROM audit_events
		WHERE tenant_id = $1
		ORDER BY occurred_at DESC
	`

	rows, err := r.db.QueryContext(context.Background(), query, tenantID)
	if err != nil {
		return nil, fmt.Errorf("querying audit events by tenant: %w", err)
	}
	defer func() { _ = rows.Close() }()

	return scanAuditEvents(rows)
}

func scanAuditEvents(rows *sql.Rows) ([]domain.AuditEvent, error) {
	var result []domain.AuditEvent

	for rows.Next() {
		var event domain.AuditEvent
		var action, targetType string
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

		if err := json.Unmarshal(metadata, &event.Metadata); err != nil {
			return nil, fmt.Errorf("unmarshaling metadata: %w", err)
		}

		result = append(result, event)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterating audit event rows: %w", err)
	}

	return result, nil
}
