package middleware

import (
	"context"
	"net/http"
	"strings"

	"audit-go/internal/platform/security"
)

// Auth validates the JWT cookie and stores the user login in context.
func Auth(jwtSvc security.TokenService) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			userLogin := extractValidLogin(jwtSvc, r)
			if userLogin == "" {
				http.Error(w, "unauthorized", http.StatusUnauthorized)
				return
			}

			ctx := context.WithValue(r.Context(), UserLoginKey, userLogin)
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

func extractValidLogin(jwtSvc security.TokenService, r *http.Request) string {
	// Expect the access token in the Authorization header: "Bearer <token>"
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

	userLogin, err := jwtSvc.ExtractUserLogin(token)
	if err != nil || userLogin == "" {
		return ""
	}

	return userLogin
}
