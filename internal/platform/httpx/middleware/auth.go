// Package middleware provides HTTP middleware used across handlers.
package middleware

import (
	"context"
	"net/http"
	"strings"

	"audit-go/internal/features/access"
	"audit-go/internal/platform/contextx"
	"audit-go/internal/platform/security"
)

// SessionResolver resolves opaque application sessions.
type SessionResolver interface {
	PrincipalFromSession(ctx context.Context, sessionToken string) (access.Principal, error)
}

// Auth validates an app session cookie first, then falls back to a Microsoft
// Entra bearer token for non-browser clients.
func Auth(
	validator *security.EntraTokenValidator,
	sessions SessionResolver,
	sessionCookieName string,
) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if sessions != nil && sessionCookieName != "" {
				if cookie, err := r.Cookie(sessionCookieName); err == nil && cookie.Value != "" {
					principal, err := sessions.PrincipalFromSession(r.Context(), cookie.Value)
					if err == nil {
						next.ServeHTTP(w, r.WithContext(withPrincipal(r.Context(), principal)))
						return
					}
				}
			}

			rawToken := extractBearerToken(r)
			if rawToken == "" {
				http.Error(w, "unauthorized", http.StatusUnauthorized)
				return
			}

			claims, err := validator.Validate(r.Context(), rawToken)
			if err != nil {
				http.Error(w, "unauthorized", http.StatusUnauthorized)
				return
			}

			principal := access.Principal{
				ID:    claims.OID,
				Login: claims.Login,
				Name:  claims.Name,
				Roles: access.RolesFromStrings(claims.Roles),
			}

			next.ServeHTTP(w, r.WithContext(withPrincipal(r.Context(), principal)))
		})
	}
}

func withPrincipal(ctx context.Context, principal access.Principal) context.Context {
	ctx = access.WithPrincipal(ctx, principal)
	ctx = contextx.Set(ctx, contextx.UserIDKey, principal.UserKey())
	ctx = contextx.Set(ctx, contextx.UserNameKey, principal.Name)
	return ctx
}

func extractBearerToken(r *http.Request) string {
	auth := r.Header.Get("Authorization")
	if auth == "" {
		return ""
	}
	parts := strings.SplitN(auth, " ", 2)
	if len(parts) != 2 || strings.ToLower(parts[0]) != "bearer" {
		return ""
	}
	return strings.TrimSpace(parts[1])
}
