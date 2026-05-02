// Package postgres implements access persistence and authorization queries.
package postgres

import (
	"context"
	"database/sql"
	"fmt"

	"github.com/google/uuid"
	"github.com/lib/pq"

	"audit-go/internal/features/access"
)

// Authorizer checks application permissions against stored memberships.
type Authorizer struct {
	db *sql.DB
}

// NewAuthorizer creates a PostgreSQL-backed authorizer.
func NewAuthorizer(db *sql.DB) *Authorizer {
	return &Authorizer{db: db}
}

// CanAccessJV checks whether principal has permission over the given joint venture.
func (a *Authorizer) CanAccessJV(
	ctx context.Context,
	principal access.Principal,
	jvID string,
	permission access.Permission,
) error {
	if !principal.Authenticated() {
		return access.ErrUnauthenticated
	}
	if _, err := uuid.Parse(jvID); err != nil {
		return fmt.Errorf("invalid joint venture id: %w", err)
	}

	roles := roleStrings(access.RolesForPermission(permission))
	if len(roles) == 0 {
		return access.ErrForbidden
	}

	const query = `
		SELECT EXISTS (
			SELECT 1
			FROM access_memberships m
			LEFT JOIN joint_ventures jv ON jv.id = $2::uuid
			WHERE m.user_login = $1
			  AND m.role = ANY($3)
			  AND (
				m.scope_type = 'system'
				OR (m.scope_type = 'joint_venture' AND m.scope_id = $2::uuid)
				OR (m.scope_type = 'region' AND m.scope_id = jv.region_id)
			  )
		)
	`

	var allowed bool
	if err := a.db.QueryRowContext(ctx, query, principal.UserKey(), jvID, pq.Array(roles)).Scan(&allowed); err != nil {
		return fmt.Errorf("checking joint venture access: %w", err)
	}
	if !allowed {
		return access.ErrForbidden
	}

	return nil
}

func roleStrings(roles []access.Role) []string {
	out := make([]string, 0, len(roles))
	for _, role := range roles {
		out = append(out, string(role))
	}
	return out
}
