// Package access owns application authorization concepts.
package access

import (
	"context"
	"errors"
	"time"
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

	PermissionRegionCreate Permission = "region:create"
	PermissionRegionRead   Permission = "region:read"
	PermissionRegionUpdate Permission = "region:update"
	PermissionRegionDelete Permission = "region:delete"

	PermissionJVCreate Permission = "joint_venture:create"
	PermissionJVRead   Permission = "joint_venture:read"
	PermissionJVUpdate Permission = "joint_venture:update"
	PermissionJVDelete Permission = "joint_venture:delete"

	PermissionMembershipManage Permission = "membership:manage"
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

// Membership grants a role to a user within a specific authorization scope.
type Membership struct {
	ID        string    `json:"id"`
	UserLogin string    `json:"user_login"`
	Role      Role      `json:"role"`
	ScopeType ScopeType `json:"scope_type"`
	ScopeID   string    `json:"scope_id,omitempty"`
	CreatedAt time.Time `json:"created_at"`
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

// IsValidRole reports whether role is supported by the application.
func IsValidRole(role Role) bool {
	switch role {
	case RoleAdmin, RoleRegionAdmin, RoleJVAdmin, RoleContributor, RoleAuditor, RoleVisitor:
		return true
	default:
		return false
	}
}

// IsValidScopeType reports whether scopeType is a supported membership scope.
func IsValidScopeType(scopeType ScopeType) bool {
	switch scopeType {
	case ScopeSystem, ScopeRegion, ScopeJointVenture:
		return true
	default:
		return false
	}
}

// RoleStrings converts roles to their database representation.
func RoleStrings(roles []Role) []string {
	out := make([]string, 0, len(roles))
	for _, role := range roles {
		out = append(out, string(role))
	}
	return out
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
	case PermissionRegionCreate:
		return []Role{RoleAdmin}
	case PermissionRegionRead:
		return []Role{RoleAdmin, RoleRegionAdmin, RoleJVAdmin, RoleContributor, RoleAuditor, RoleVisitor}
	case PermissionRegionUpdate:
		return []Role{RoleAdmin, RoleRegionAdmin}
	case PermissionRegionDelete:
		return []Role{RoleAdmin}
	case PermissionJVCreate:
		return []Role{RoleAdmin, RoleRegionAdmin}
	case PermissionJVRead:
		return []Role{RoleAdmin, RoleRegionAdmin, RoleJVAdmin, RoleContributor, RoleAuditor, RoleVisitor}
	case PermissionJVUpdate:
		return []Role{RoleAdmin, RoleRegionAdmin, RoleJVAdmin}
	case PermissionJVDelete:
		return []Role{RoleAdmin, RoleRegionAdmin}
	case PermissionMembershipManage:
		return []Role{RoleAdmin, RoleRegionAdmin, RoleJVAdmin}
	default:
		return nil
	}
}
