// Package access owns application authorization concepts.
package access

import (
	"context"
	"errors"
)

type principalKey struct{}

var (
	// ErrUnauthenticated means no application principal is available.
	ErrUnauthenticated = errors.New("access: unauthenticated")
	// ErrForbidden means the principal does not have the required permission.
	ErrForbidden = errors.New("access: forbidden")
)

// Role is an application role. Entra proves identity; these roles authorize app resources.
type Role string

const (
	RoleAdmin       Role = "admin"
	RoleRegionAdmin Role = "region_admin"
	RoleJVAdmin     Role = "jv_admin"
	RoleContributor Role = "contributor"
	RoleAuditor     Role = "auditor"
	RoleVisitor     Role = "visitor"
)

// Permission names the application action being authorized.
type Permission string

const (
	PermissionDocumentCreate Permission = "document:create"
	PermissionDocumentRead   Permission = "document:read"
	PermissionDocumentDelete Permission = "document:delete"
)

// ScopeType describes where a role applies.
type ScopeType string

const (
	ScopeSystem       ScopeType = "system"
	ScopeRegion       ScopeType = "region"
	ScopeJointVenture ScopeType = "joint_venture"
)

// Principal is the authenticated application actor.
type Principal struct {
	ID    string
	Login string
	Name  string
	Roles []Role
}

// UserKey returns the stable key used by access membership rows.
func (p Principal) UserKey() string {
	if p.Login != "" {
		return p.Login
	}
	return p.ID
}

// Authenticated reports whether the principal represents a signed-in user.
func (p Principal) Authenticated() bool {
	return p.UserKey() != ""
}

// HasRole reports whether a role is present in token-derived roles.
func (p Principal) HasRole(role Role) bool {
	for _, r := range p.Roles {
		if r == role {
			return true
		}
	}
	return false
}

// WithPrincipal stores p in ctx.
func WithPrincipal(ctx context.Context, p Principal) context.Context {
	return context.WithValue(ctx, principalKey{}, p)
}

// PrincipalFromContext returns the authenticated principal stored in ctx.
func PrincipalFromContext(ctx context.Context) (Principal, bool) {
	p, ok := ctx.Value(principalKey{}).(Principal)
	return p, ok && p.Authenticated()
}

// RolesFromStrings converts external role claim strings into application roles.
func RolesFromStrings(values []string) []Role {
	roles := make([]Role, 0, len(values))
	for _, value := range values {
		switch Role(value) {
		case RoleAdmin, RoleRegionAdmin, RoleJVAdmin, RoleContributor, RoleAuditor, RoleVisitor:
			roles = append(roles, Role(value))
		}
	}
	return roles
}

// RolesForPermission returns roles that may satisfy a permission within a valid scope.
func RolesForPermission(permission Permission) []Role {
	switch permission {
	case PermissionDocumentCreate:
		return []Role{RoleAdmin, RoleRegionAdmin, RoleJVAdmin, RoleContributor}
	case PermissionDocumentRead:
		return []Role{RoleAdmin, RoleRegionAdmin, RoleJVAdmin, RoleContributor, RoleAuditor, RoleVisitor}
	case PermissionDocumentDelete:
		return []Role{RoleAdmin, RoleRegionAdmin, RoleJVAdmin}
	default:
		return nil
	}
}
