package middleware

import (
	"context"
	"errors"
	"fmt"
	"net/http"

	"github.com/google/uuid"

	"audit-go/internal/features/access"
	"audit-go/internal/platform/httpx"
)

// PermissionAuthorizer checks scoped permissions for HTTP routes.
type PermissionAuthorizer interface {
	CanAccessSystem(ctx context.Context, principal access.Principal, permission access.Permission) error
	CanAccessRegion(ctx context.Context, principal access.Principal, regionID string, permission access.Permission) error
	CanAccessJV(ctx context.Context, principal access.Principal, jvID string, permission access.Permission) error
}

// AuthorizationScope identifies the target scope for a permission check.
type AuthorizationScope struct {
	Type access.ScopeType
	ID   string
}

// ScopeResolver derives the authorization scope from an HTTP request.
type ScopeResolver func(*http.Request) (AuthorizationScope, error)

// RequirePermission authorizes a request after authentication has attached a principal.
func RequirePermission(
	authorizer PermissionAuthorizer,
	permission access.Permission,
	resolve ScopeResolver,
) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if authorizer == nil || resolve == nil {
				if err := httpx.WriteError(w, http.StatusInternalServerError, "authorization is not configured"); err != nil {
					http.Error(w, "internal server error", http.StatusInternalServerError)
				}
				return
			}

			principal, ok := access.PrincipalFromContext(r.Context())
			if !ok {
				if err := httpx.WriteError(w, http.StatusUnauthorized, "unauthorized"); err != nil {
					http.Error(w, "unauthorized", http.StatusUnauthorized)
				}
				return
			}

			scope, err := resolve(r)
			if err != nil {
				if writeErr := httpx.WriteError(w, http.StatusBadRequest, err.Error()); writeErr != nil {
					http.Error(w, "bad request", http.StatusBadRequest)
				}
				return
			}

			if err = authorize(r, authorizer, principal, permission, scope); err != nil {
				writeAuthorizationError(w, err)
				return
			}

			next.ServeHTTP(w, r)
		})
	}
}

// SystemScope resolves a system-scoped authorization target.
func SystemScope() ScopeResolver {
	return func(*http.Request) (AuthorizationScope, error) {
		return AuthorizationScope{Type: access.ScopeSystem}, nil
	}
}

// RegionScopeFromPath resolves a region id from a path variable.
func RegionScopeFromPath(param string) ScopeResolver {
	return func(r *http.Request) (AuthorizationScope, error) {
		id := r.PathValue(param)
		if id == "" {
			return AuthorizationScope{}, fmt.Errorf("%s is required", param)
		}
		if _, err := uuid.Parse(id); err != nil {
			return AuthorizationScope{}, fmt.Errorf("invalid %s", param)
		}
		return AuthorizationScope{Type: access.ScopeRegion, ID: id}, nil
	}
}

// JVScopeFromPath resolves a joint venture id from a path variable.
func JVScopeFromPath(param string) ScopeResolver {
	return func(r *http.Request) (AuthorizationScope, error) {
		id := r.PathValue(param)
		if id == "" {
			return AuthorizationScope{}, fmt.Errorf("%s is required", param)
		}
		if _, err := uuid.Parse(id); err != nil {
			return AuthorizationScope{}, fmt.Errorf("invalid %s", param)
		}
		return AuthorizationScope{Type: access.ScopeJointVenture, ID: id}, nil
	}
}

func authorize(
	r *http.Request,
	authorizer PermissionAuthorizer,
	principal access.Principal,
	permission access.Permission,
	scope AuthorizationScope,
) error {
	switch scope.Type {
	case access.ScopeSystem:
		return authorizer.CanAccessSystem(r.Context(), principal, permission)
	case access.ScopeRegion:
		return authorizer.CanAccessRegion(r.Context(), principal, scope.ID, permission)
	case access.ScopeJointVenture:
		return authorizer.CanAccessJV(r.Context(), principal, scope.ID, permission)
	default:
		return access.ErrForbidden
	}
}

func writeAuthorizationError(w http.ResponseWriter, err error) {
	status := http.StatusInternalServerError
	message := "internal server error"

	switch {
	case errors.Is(err, access.ErrUnauthenticated):
		status = http.StatusUnauthorized
		message = "unauthorized"
	case errors.Is(err, access.ErrForbidden):
		status = http.StatusForbidden
		message = "forbidden"
	}

	if writeErr := httpx.WriteError(w, status, message); writeErr != nil {
		http.Error(w, message, status)
	}
}
