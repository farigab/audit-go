// Package middleware provides HTTP middleware used across handlers.
package middleware

import (
	"net/http"

	"github.com/google/uuid"

	"audit-go/internal/platform/contextx"
)

// RequestContext injects request-scoped values into context.
func RequestContext(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requestID := uuid.NewString()

		tenantID := r.Header.Get("X-Tenant-ID")
		userID := r.Header.Get("X-User-ID")

		ctx := r.Context()

		ctx = contextx.Set(ctx, contextx.RequestIDKey, requestID)
		ctx = contextx.Set(ctx, contextx.TenantIDKey, tenantID)
		ctx = contextx.Set(ctx, contextx.UserIDKey, userID)

		next.ServeHTTP(w, r.WithContext(ctx))
	})
}
