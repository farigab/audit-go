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

		ctx := r.Context()
		ctx = contextx.Set(ctx, contextx.RequestIDKey, requestID)

		userID := r.Header.Get("X-User-ID")
		tenantID := r.Header.Get("X-Tenant-ID")

		if userID != "" {
			ctx = contextx.Set(ctx, contextx.UserIDKey, userID)
		}

		if tenantID != "" {
			ctx = contextx.Set(ctx, contextx.TenantIDKey, tenantID)
		}

		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

type responseWriter struct {
	http.ResponseWriter
	status int
}

func (rw *responseWriter) WriteHeader(status int) {
	rw.status = status
	rw.ResponseWriter.WriteHeader(status)
}

func Logging(log zerolog.Logger, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()

		rw := &responseWriter{
			ResponseWriter: w,
			status:         http.StatusOK,
		}

		next.ServeHTTP(rw, r)

		ctx := r.Context()
		elapsed := time.Since(start)

		event := log.Info().
			Str("request_id", contextx.Get(ctx, contextx.RequestIDKey)).
			Str("method", r.Method).
			Str("path", r.URL.Path).
			Int("status", rw.status).
			Int64("duration_us", elapsed.Microseconds())

		if userID := contextx.Get(ctx, contextx.UserIDKey); userID != "" {
			event.Str("user_id", userID)
		}

		if tenantID := contextx.Get(ctx, contextx.TenantIDKey); tenantID != "" {
			event.Str("tenant_id", tenantID)
		}

		event.Msg("http request")
	})
}
