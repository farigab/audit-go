// Package middleware provides HTTP middleware used across handlers.
package middleware

import (
	"net/http"
	"strings"

	"audit-go/internal/features/access"
	"audit-go/internal/platform/contextx"
	"audit-go/internal/platform/security"
)

// Auth validates the Bearer token issued by Microsoft Entra ID and stores the
// application principal in context.
func Auth(validator *security.EntraTokenValidator) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
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

			ctx := access.WithPrincipal(r.Context(), principal)
			ctx = contextx.Set(ctx, contextx.UserIDKey, principal.UserKey())
			ctx = contextx.Set(ctx, contextx.UserNameKey, claims.Name)

			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
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
