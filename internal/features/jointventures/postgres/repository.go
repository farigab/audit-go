// Package postgres implements joint venture persistence.
package postgres

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/lib/pq"

	"audit-go/internal/features/access"
	"audit-go/internal/features/jointventures"
	platformpostgres "audit-go/internal/platform/postgres"
)

// Repository stores joint ventures in PostgreSQL.
type Repository struct {
	db *sql.DB
}

// NewRepository creates a PostgreSQL joint venture repository.
func NewRepository(db *sql.DB) *Repository {
	return &Repository{db: db}
}

// Save inserts a joint venture.
func (r *Repository) Save(ctx context.Context, jv jointventures.JointVenture) error {
	metadata, err := json.Marshal(jv.Metadata)
	if err != nil {
		return fmt.Errorf("marshaling joint venture metadata: %w", err)
	}

	const query = `
		INSERT INTO joint_ventures (
			id,
			region_id,
			name,
			parties,
			status,
			created_by,
			created_at,
			updated_at,
			metadata
		)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9::jsonb)
	`

	_, err = platformpostgres.Executor(ctx, r.db).ExecContext(
		ctx,
		query,
		jv.ID,
		jv.RegionID,
		jv.Name,
		pq.Array(jv.Parties),
		string(jv.Status),
		jv.CreatedBy,
		jv.CreatedAt,
		jv.UpdatedAt,
		metadata,
	)
	if err != nil {
		return fmt.Errorf("saving joint venture: %w", err)
	}

	return nil
}

// FindByID returns a joint venture by id.
func (r *Repository) FindByID(ctx context.Context, id string) (*jointventures.JointVenture, error) {
	const query = `
		SELECT
			id,
			region_id,
			name,
			parties,
			status,
			created_by,
			created_at,
			updated_at,
			metadata
		FROM joint_ventures
		WHERE id = $1
	`

	jv, err := scanJointVenture(platformpostgres.Executor(ctx, r.db).QueryRowContext(ctx, query, id))
	if errors.Is(err, sql.ErrNoRows) {
		return nil, errors.New("joint venture not found")
	}
	if err != nil {
		return nil, fmt.Errorf("finding joint venture: %w", err)
	}

	return jv, nil
}

// ListByRegionAccessible returns JVs in a region visible to actor.
func (r *Repository) ListByRegionAccessible(
	ctx context.Context,
	regionID string,
	actor access.Principal,
) ([]jointventures.JointVenture, error) {
	const query = `
		SELECT DISTINCT
			jv.id,
			jv.region_id,
			jv.name,
			jv.parties,
			jv.status,
			jv.created_by,
			jv.created_at,
			jv.updated_at,
			jv.metadata
		FROM joint_ventures jv
		JOIN access_memberships m ON m.user_login = $1
		WHERE jv.region_id = $2
		  AND m.role = ANY($3)
		  AND (
			m.scope_type = 'system'
			OR (m.scope_type = 'region' AND m.scope_id = jv.region_id)
			OR (m.scope_type = 'joint_venture' AND m.scope_id = jv.id)
		  )
		ORDER BY jv.name ASC
	`

	rows, err := platformpostgres.Executor(ctx, r.db).QueryContext(
		ctx,
		query,
		actor.UserKey(),
		regionID,
		pq.Array(access.RoleStrings(access.RolesForPermission(access.PermissionJVRead))),
	)
	if err != nil {
		return nil, fmt.Errorf("querying accessible joint ventures by region: %w", err)
	}
	defer func() { _ = rows.Close() }()

	return scanJointVentures(rows)
}

// Update replaces mutable JV fields.
func (r *Repository) Update(ctx context.Context, jv jointventures.JointVenture) error {
	metadata, err := json.Marshal(jv.Metadata)
	if err != nil {
		return fmt.Errorf("marshaling joint venture metadata: %w", err)
	}

	const query = `
		UPDATE joint_ventures
		SET name = $2,
			parties = $3,
			status = $4,
			metadata = $5::jsonb,
			updated_at = NOW()
		WHERE id = $1
	`

	res, err := platformpostgres.Executor(ctx, r.db).ExecContext(
		ctx,
		query,
		jv.ID,
		jv.Name,
		pq.Array(jv.Parties),
		string(jv.Status),
		metadata,
	)
	if err != nil {
		return fmt.Errorf("updating joint venture: %w", err)
	}
	rows, err := res.RowsAffected()
	if err != nil {
		return fmt.Errorf("checking updated joint venture rows: %w", err)
	}
	if rows == 0 {
		return errors.New("joint venture not found")
	}

	return nil
}

// Delete removes a joint venture by id.
func (r *Repository) Delete(ctx context.Context, id string) error {
	res, err := platformpostgres.Executor(ctx, r.db).ExecContext(ctx, `DELETE FROM joint_ventures WHERE id = $1`, id)
	if err != nil {
		return fmt.Errorf("deleting joint venture: %w", err)
	}
	rows, err := res.RowsAffected()
	if err != nil {
		return fmt.Errorf("checking deleted joint venture rows: %w", err)
	}
	if rows == 0 {
		return errors.New("joint venture not found")
	}

	return nil
}

type jointVentureScanner interface {
	Scan(dest ...any) error
}

func scanJointVenture(scanner jointVentureScanner) (*jointventures.JointVenture, error) {
	var jv jointventures.JointVenture
	var status string
	var parties pq.StringArray
	var createdAt time.Time
	var updatedAt time.Time
	var metadata []byte

	if err := scanner.Scan(
		&jv.ID,
		&jv.RegionID,
		&jv.Name,
		&parties,
		&status,
		&jv.CreatedBy,
		&createdAt,
		&updatedAt,
		&metadata,
	); err != nil {
		return nil, err
	}

	jv.Parties = []string(parties)
	jv.Status = jointventures.Status(status)
	jv.CreatedAt = createdAt.UTC()
	jv.UpdatedAt = updatedAt.UTC()
	jv.Metadata = map[string]string{}
	if len(metadata) > 0 {
		if err := json.Unmarshal(metadata, &jv.Metadata); err != nil {
			return nil, fmt.Errorf("unmarshaling joint venture metadata: %w", err)
		}
	}

	return &jv, nil
}

func scanJointVentures(rows *sql.Rows) ([]jointventures.JointVenture, error) {
	items := make([]jointventures.JointVenture, 0)
	for rows.Next() {
		jv, err := scanJointVenture(rows)
		if err != nil {
			return nil, fmt.Errorf("scanning joint venture row: %w", err)
		}
		items = append(items, *jv)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterating joint venture rows: %w", err)
	}

	return items, nil
}
