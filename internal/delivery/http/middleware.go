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
		ctx = contextx.Set(ctx, contextx.RequestIDKey, requestID)
		ctx = contextx.Set(ctx, contextx.UserIDKey, userID)
		ctx = contextx.Set(ctx, contextx.TenantIDKey, tenantID)

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
		rw := &responseWriter{ResponseWriter: w, status: http.StatusOK}

		next.ServeHTTP(rw, r)

		log.Info().
			Str("request_id", contextx.Get(r.Context(), contextx.RequestIDKey)).
			Str("user_id", contextx.Get(r.Context(), contextx.UserIDKey)).
			Str("tenant_id", contextx.Get(r.Context(), contextx.TenantIDKey)).
			Str("method", r.Method).
			Str("path", r.URL.Path).
			Int("status", rw.status).
			Dur("duration", time.Since(start)).
			Msg("http request")
	})
}
