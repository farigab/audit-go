package postgres

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"

	"audit-go/internal/features/access"
	accessapp "audit-go/internal/features/access/app"
)

// MembershipRepository persists access memberships in PostgreSQL.
type MembershipRepository struct {
	db *sql.DB
}

// NewMembershipRepository creates a PostgreSQL-backed membership repository.
func NewMembershipRepository(db *sql.DB) *MembershipRepository {
	return &MembershipRepository{db: db}
}

// SaveMembership inserts a role grant.
func (r *MembershipRepository) SaveMembership(ctx context.Context, membership access.Membership) error {
	const query = `
		INSERT INTO access_memberships (
			id,
			user_login,
			role,
			scope_type,
			scope_id,
			created_at
		)
		VALUES ($1,$2,$3,$4,$5,$6)
	`

	createdAt := membership.CreatedAt
	if createdAt.IsZero() {
		createdAt = time.Now().UTC()
	}

	_, err := r.db.ExecContext(
		ctx,
		query,
		membership.ID,
		membership.UserLogin,
		string(membership.Role),
		string(membership.ScopeType),
		nullableString(membership.ScopeID),
		createdAt,
	)
	if err != nil {
		return fmt.Errorf("saving access membership: %w", err)
	}

	return nil
}

// FindMembershipByID returns one membership by id.
func (r *MembershipRepository) FindMembershipByID(ctx context.Context, id string) (*access.Membership, error) {
	const query = `
		SELECT
			id,
			user_login,
			role,
			scope_type,
			scope_id,
			created_at
		FROM access_memberships
		WHERE id = $1
	`

	membership, err := scanMembership(r.db.QueryRowContext(ctx, query, id))
	if errors.Is(err, sql.ErrNoRows) {
		return nil, errors.New("membership not found")
	}
	if err != nil {
		return nil, fmt.Errorf("finding access membership: %w", err)
	}

	return membership, nil
}

// ListMemberships returns memberships matching the provided filter.
func (r *MembershipRepository) ListMemberships(
	ctx context.Context,
	filter accessapp.MembershipFilter,
) ([]access.Membership, error) {
	query := strings.Builder{}
	query.WriteString(`
		SELECT
			id,
			user_login,
			role,
			scope_type,
			scope_id,
			created_at
		FROM access_memberships
		WHERE 1 = 1
	`)

	args := make([]any, 0, 3)
	if filter.UserLogin != "" {
		args = append(args, filter.UserLogin)
		query.WriteString(fmt.Sprintf(" AND user_login = $%d", len(args)))
	}
	if filter.ScopeType != "" {
		args = append(args, string(filter.ScopeType))
		query.WriteString(fmt.Sprintf(" AND scope_type = $%d", len(args)))
		if filter.ScopeType == access.ScopeSystem {
			query.WriteString(" AND scope_id IS NULL")
		} else if filter.ScopeID != "" {
			args = append(args, filter.ScopeID)
			query.WriteString(fmt.Sprintf(" AND scope_id = $%d::uuid", len(args)))
		}
	}
	query.WriteString(" ORDER BY created_at DESC")

	rows, err := r.db.QueryContext(ctx, query.String(), args...)
	if err != nil {
		return nil, fmt.Errorf("querying access memberships: %w", err)
	}
	defer func() { _ = rows.Close() }()

	return scanMemberships(rows)
}

// DeleteMembership removes a role grant.
func (r *MembershipRepository) DeleteMembership(ctx context.Context, id string) error {
	res, err := r.db.ExecContext(ctx, `DELETE FROM access_memberships WHERE id = $1`, id)
	if err != nil {
		return fmt.Errorf("deleting access membership: %w", err)
	}
	rows, err := res.RowsAffected()
	if err != nil {
		return fmt.Errorf("checking deleted membership rows: %w", err)
	}
	if rows == 0 {
		return errors.New("membership not found")
	}

	return nil
}

type membershipScanner interface {
	Scan(dest ...any) error
}

func scanMembership(scanner membershipScanner) (*access.Membership, error) {
	var membership access.Membership
	var role string
	var scopeType string
	var scopeID sql.NullString

	if err := scanner.Scan(
		&membership.ID,
		&membership.UserLogin,
		&role,
		&scopeType,
		&scopeID,
		&membership.CreatedAt,
	); err != nil {
		return nil, err
	}

	membership.Role = access.Role(role)
	membership.ScopeType = access.ScopeType(scopeType)
	if scopeID.Valid {
		membership.ScopeID = scopeID.String
	}
	membership.CreatedAt = membership.CreatedAt.UTC()

	return &membership, nil
}

func scanMemberships(rows *sql.Rows) ([]access.Membership, error) {
	memberships := make([]access.Membership, 0)
	for rows.Next() {
		membership, err := scanMembership(rows)
		if err != nil {
			return nil, fmt.Errorf("scanning access membership row: %w", err)
		}
		memberships = append(memberships, *membership)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterating access membership rows: %w", err)
	}

	return memberships, nil
}
