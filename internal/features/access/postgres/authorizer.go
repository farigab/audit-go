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

// CanAccessSystem checks whether principal has a system-scoped permission.
func (a *Authorizer) CanAccessSystem(
	ctx context.Context,
	principal access.Principal,
	permission access.Permission,
) error {
	if !principal.Authenticated() {
		return access.ErrUnauthenticated
	}

	roles := access.RoleStrings(access.RolesForPermission(permission))
	if len(roles) == 0 {
		return access.ErrForbidden
	}

	const query = `
		SELECT EXISTS (
			SELECT 1
			FROM access_memberships m
			WHERE m.user_login = $1
			  AND m.role = ANY($2)
			  AND m.scope_type = 'system'
		)
	`

	var allowed bool
	if err := a.db.QueryRowContext(ctx, query, principal.UserKey(), pq.Array(roles)).Scan(&allowed); err != nil {
		return fmt.Errorf("checking system access: %w", err)
	}
	if !allowed {
		return access.ErrForbidden
	}

	return nil
}

// CanAccessRegion checks whether principal has permission over a region.
func (a *Authorizer) CanAccessRegion(
	ctx context.Context,
	principal access.Principal,
	regionID string,
	permission access.Permission,
) error {
	if !principal.Authenticated() {
		return access.ErrUnauthenticated
	}
	if _, err := uuid.Parse(regionID); err != nil {
		return fmt.Errorf("invalid region id: %w", err)
	}

	roles := access.RoleStrings(access.RolesForPermission(permission))
	if len(roles) == 0 {
		return access.ErrForbidden
	}

	const query = `
		SELECT EXISTS (
			SELECT 1
			FROM access_memberships m
			WHERE m.user_login = $1
			  AND m.role = ANY($3)
			  AND (
				m.scope_type = 'system'
				OR (m.scope_type = 'region' AND m.scope_id = $2::uuid)
			  )
		)
	`

	var allowed bool
	if err := a.db.QueryRowContext(ctx, query, principal.UserKey(), regionID, pq.Array(roles)).Scan(&allowed); err != nil {
		return fmt.Errorf("checking region access: %w", err)
	}
	if !allowed {
		return access.ErrForbidden
	}

	return nil
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

	roles := access.RoleStrings(access.RolesForPermission(permission))
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

// CanManageMembership checks whether principal may mutate memberships in the target scope.
func (a *Authorizer) CanManageMembership(
	ctx context.Context,
	principal access.Principal,
	scopeType access.ScopeType,
	scopeID string,
) error {
	if scopeType == access.ScopeSystem {
		return a.canManageSystemMembership(ctx, principal)
	}
	if scopeType == access.ScopeRegion {
		return a.canManageRegionMembership(ctx, principal, scopeID)
	}
	if scopeType == access.ScopeJointVenture {
		return a.CanAccessJV(ctx, principal, scopeID, access.PermissionMembershipManage)
	}
	return access.ErrForbidden
}

func (a *Authorizer) canManageRegionMembership(ctx context.Context, principal access.Principal, regionID string) error {
	if !principal.Authenticated() {
		return access.ErrUnauthenticated
	}
	if _, err := uuid.Parse(regionID); err != nil {
		return fmt.Errorf("invalid region id: %w", err)
	}

	const query = `
		SELECT EXISTS (
			SELECT 1
			FROM access_memberships m
			WHERE m.user_login = $1
			  AND m.role = ANY($3)
			  AND (
				m.scope_type = 'system'
				OR (m.scope_type = 'region' AND m.scope_id = $2::uuid)
			  )
		)
	`

	roles := []string{string(access.RoleAdmin), string(access.RoleRegionAdmin)}
	var allowed bool
	if err := a.db.QueryRowContext(ctx, query, principal.UserKey(), regionID, pq.Array(roles)).Scan(&allowed); err != nil {
		return fmt.Errorf("checking region membership management access: %w", err)
	}
	if !allowed {
		return access.ErrForbidden
	}

	return nil
}

func (a *Authorizer) canManageSystemMembership(ctx context.Context, principal access.Principal) error {
	if !principal.Authenticated() {
		return access.ErrUnauthenticated
	}

	const query = `
		SELECT EXISTS (
			SELECT 1
			FROM access_memberships m
			WHERE m.user_login = $1
			  AND m.role = $2
			  AND m.scope_type = 'system'
		)
	`

	var allowed bool
	if err := a.db.QueryRowContext(ctx, query, principal.UserKey(), string(access.RoleAdmin)).Scan(&allowed); err != nil {
		return fmt.Errorf("checking system membership management access: %w", err)
	}
	if !allowed {
		return access.ErrForbidden
	}

	return nil
}
