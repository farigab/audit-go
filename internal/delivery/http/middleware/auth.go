package middleware

import (
	"context"
	"net/http"

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
	cookie, err := r.Cookie("token")
	if err != nil || cookie.Value == "" {
		return ""
	}

	userLogin, err := jwtSvc.ExtractUserLogin(cookie.Value)
	if err != nil || userLogin == "" {
		return ""
	}

	return userLogin
}
