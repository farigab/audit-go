package http

import (
	"net/http"
	"time"

	"github.com/google/uuid"
	"github.com/rs/zerolog"

	"audit-go/internal/platform/contextx"
)

func RequestContext(log zerolog.Logger, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requestID := uuid.NewString()

		userID := r.Header.Get("X-User-ID")
		tenantID := r.Header.Get("X-Tenant-ID")

		ctx := r.Context()
		ctx = contextx.Set(ctx, "request_id", requestID)
		ctx = contextx.Set(ctx, "user_id", userID)
		ctx = contextx.Set(ctx, "tenant_id", tenantID)

		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

func Logging(log zerolog.Logger, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()

		next.ServeHTTP(w, r)

		log.Info().
			Str("request_id", contextx.Get(r.Context(), "request_id")).
			Str("user_id", contextx.Get(r.Context(), "user_id")).
			Str("tenant_id", contextx.Get(r.Context(), "tenant_id")).
			Str("method", r.Method).
			Str("path", r.URL.Path).
			Dur("duration", time.Since(start)).
			Msg("http request")
	})
}
