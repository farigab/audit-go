// Package postgres implements region persistence.
package postgres

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/lib/pq"

	"audit-go/internal/features/access"
	"audit-go/internal/features/regions"
	platformpostgres "audit-go/internal/platform/postgres"
)

// Repository stores regions in PostgreSQL.
type Repository struct {
	db *sql.DB
}

// NewRepository creates a PostgreSQL region repository.
func NewRepository(db *sql.DB) *Repository {
	return &Repository{db: db}
}

// Save inserts a region.
func (r *Repository) Save(ctx context.Context, region regions.Region) error {
	const query = `
		INSERT INTO regions (id, name, code, created_at)
		VALUES ($1,$2,$3,$4)
	`

	_, err := platformpostgres.Executor(ctx, r.db).ExecContext(
		ctx,
		query,
		region.ID,
		region.Name,
		region.Code,
		region.CreatedAt,
	)
	if err != nil {
		return fmt.Errorf("saving region: %w", err)
	}

	return nil
}

// FindByID returns a region by id.
func (r *Repository) FindByID(ctx context.Context, id string) (*regions.Region, error) {
	const query = `
		SELECT id, name, code, created_at
		FROM regions
		WHERE id = $1
	`

	region, err := scanRegion(platformpostgres.Executor(ctx, r.db).QueryRowContext(ctx, query, id))
	if errors.Is(err, sql.ErrNoRows) {
		return nil, errors.New("region not found")
	}
	if err != nil {
		return nil, fmt.Errorf("finding region: %w", err)
	}

	return region, nil
}

// ListAccessible returns regions reachable through system or region memberships.
func (r *Repository) ListAccessible(ctx context.Context, actor access.Principal) ([]regions.Region, error) {
	const query = `
		SELECT DISTINCT r.id, r.name, r.code, r.created_at
		FROM regions r
		JOIN access_memberships m ON m.user_login = $1
		WHERE m.role = ANY($2)
		  AND (
			m.scope_type = 'system'
			OR (m.scope_type = 'region' AND m.scope_id = r.id)
			OR (
				m.scope_type = 'joint_venture'
				AND EXISTS (
					SELECT 1
					FROM joint_ventures jv
					WHERE jv.id = m.scope_id
					  AND jv.region_id = r.id
				)
			)
		  )
		ORDER BY r.code ASC, r.name ASC
	`

	rows, err := platformpostgres.Executor(ctx, r.db).QueryContext(
		ctx,
		query,
		actor.UserKey(),
		pq.Array(access.RoleStrings(access.RolesForPermission(access.PermissionRegionRead))),
	)
	if err != nil {
		return nil, fmt.Errorf("querying accessible regions: %w", err)
	}
	defer func() { _ = rows.Close() }()

	return scanRegions(rows)
}

// Update replaces mutable region fields.
func (r *Repository) Update(ctx context.Context, region regions.Region) error {
	const query = `
		UPDATE regions
		SET name = $2,
			code = $3
		WHERE id = $1
	`

	res, err := platformpostgres.Executor(ctx, r.db).ExecContext(ctx, query, region.ID, region.Name, region.Code)
	if err != nil {
		return fmt.Errorf("updating region: %w", err)
	}
	rows, err := res.RowsAffected()
	if err != nil {
		return fmt.Errorf("checking updated region rows: %w", err)
	}
	if rows == 0 {
		return errors.New("region not found")
	}

	return nil
}

// Delete removes a region.
func (r *Repository) Delete(ctx context.Context, id string) error {
	res, err := platformpostgres.Executor(ctx, r.db).ExecContext(ctx, `DELETE FROM regions WHERE id = $1`, id)
	if err != nil {
		return fmt.Errorf("deleting region: %w", err)
	}
	rows, err := res.RowsAffected()
	if err != nil {
		return fmt.Errorf("checking deleted region rows: %w", err)
	}
	if rows == 0 {
		return errors.New("region not found")
	}

	return nil
}

type regionScanner interface {
	Scan(dest ...any) error
}

func scanRegion(scanner regionScanner) (*regions.Region, error) {
	var region regions.Region
	var createdAt time.Time

	if err := scanner.Scan(&region.ID, &region.Name, &region.Code, &createdAt); err != nil {
		return nil, err
	}
	region.CreatedAt = createdAt.UTC()

	return &region, nil
}

func scanRegions(rows *sql.Rows) ([]regions.Region, error) {
	items := make([]regions.Region, 0)
	for rows.Next() {
		region, err := scanRegion(rows)
		if err != nil {
			return nil, fmt.Errorf("scanning region row: %w", err)
		}
		items = append(items, *region)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterating region rows: %w", err)
	}

	return items, nil
}
