package middleware

import (
	"net/http"

	"github.com/google/uuid"

	"audit-go/internal/platform/contextx"
)

// RequestContext injects request-scoped values into context.
func RequestContext(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ctx := contextx.Set(r.Context(), contextx.RequestIDKey, uuid.NewString())
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}
