// Package middleware provides HTTP middleware used across handlers.
package middleware

import (
	"context"
	"net/http"
	"strings"

	"audit-go/internal/platform/contextx"
	"audit-go/internal/platform/security"
)

type contextKey string

const UserLoginKey contextKey = "userLogin"

// UserLoginFromContext extracts the authenticated user login from context.
func UserLoginFromContext(ctx context.Context) (string, bool) {
	login, ok := ctx.Value(UserLoginKey).(string)
	return login, ok && login != ""
}

// Auth validates the Bearer token issued by Microsoft Entra ID and stores the
// user login (preferred_username / OID) in context.
//
// Token rotation is handled by Entra ID — this middleware only validates.
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

			ctx := context.WithValue(r.Context(), UserLoginKey, claims.Login)
			ctx = contextx.Set(ctx, contextx.UserIDKey, claims.Login)

			// Optionally expose display name for logging
			ctx = contextx.Set(ctx, contextx.UserNameKey, claims.Name)

			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

// extractBearerToken reads the Authorization header and returns the raw token.
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
