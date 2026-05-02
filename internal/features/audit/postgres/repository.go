// Package postgres implements audit persistence.
package postgres

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"

	"audit-go/internal/features/audit"
	platformpostgres "audit-go/internal/platform/postgres"
)

// Repository persists audit events in PostgreSQL.
type Repository struct {
	db *sql.DB
}

// NewRepository creates a PostgreSQL audit repository.
func NewRepository(db *sql.DB) *Repository {
	return &Repository{db: db}
}

// Save persists an audit event.
func (r *Repository) Save(ctx context.Context, event audit.Event) error {
	metadata, err := json.Marshal(event.Metadata)
	if err != nil {
		return fmt.Errorf("marshaling metadata: %w", err)
	}

	const query = `
		INSERT INTO audit_events (
			id,
			actor_id,
			action,
			target_id,
			target_type,
			occurred_at,
			request_id,
			metadata
		)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8)
	`

	_, err = platformpostgres.Executor(ctx, r.db).ExecContext(
		ctx,
		query,
		event.ID,
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
func (r *Repository) FindByTarget(ctx context.Context, targetID string) ([]audit.Event, error) {
	const query = `
		SELECT
			id,
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

	rows, err := platformpostgres.Executor(ctx, r.db).QueryContext(ctx, query, targetID)
	if err != nil {
		return nil, fmt.Errorf("querying audit events by target: %w", err)
	}
	defer func() { _ = rows.Close() }()

	return scanEvents(rows)
}

func scanEvents(rows *sql.Rows) ([]audit.Event, error) {
	var events []audit.Event

	for rows.Next() {
		var event audit.Event
		var action string
		var targetType string
		var metadata []byte

		if err := rows.Scan(
			&event.ID,
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

		event.Action = audit.Action(action)
		event.TargetType = audit.TargetType(targetType)
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
