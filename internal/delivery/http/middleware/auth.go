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

// Auth validates the Bearer JWT in the Authorization header and stores the
// user login in context. Use this for all authenticated routes.
//
// Token rotation is handled explicitly by POST /auth/refresh — this
// middleware never rotates automatically.
func Auth(jwtSvc security.TokenService) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			login := extractValidLogin(jwtSvc, r)
			if login == "" {
				http.Error(w, "unauthorized", http.StatusUnauthorized)
				return
			}

			ctx := context.WithValue(r.Context(), UserLoginKey, login)
			ctx = contextx.Set(ctx, contextx.UserIDKey, login)
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

// extractValidLogin parses and validates the Bearer token from the
// Authorization header, returning the user login on success or an empty
// string on any failure.
func extractValidLogin(jwtSvc security.TokenService, r *http.Request) string {
	auth := r.Header.Get("Authorization")
	if auth == "" {
		return ""
	}

	parts := strings.SplitN(auth, " ", 2)
	if len(parts) != 2 || strings.ToLower(parts[0]) != "bearer" {
		return ""
	}

	token := strings.TrimSpace(parts[1])
	if token == "" {
		return ""
	}

	login, err := jwtSvc.ExtractUserLogin(token)
	if err != nil || login == "" {
		return ""
	}

	return login
}
