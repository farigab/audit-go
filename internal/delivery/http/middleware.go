package http

import (
	"net/http"
	"time"

	"github.com/google/uuid"
	"github.com/rs/zerolog"

	"audit-go/internal/platform/contextx"
)

// RequestContext returns a middleware that injects request-scoped values.
func RequestContext(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requestID := uuid.NewString()
		tenantID := r.Header.Get("X-Tenant-ID")

		ctx := r.Context()
		ctx = contextx.Set(ctx, contextx.RequestIDKey, requestID)
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

// Logging logs incoming HTTP requests using zerolog.
func Logging(log zerolog.Logger, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		rw := &responseWriter{ResponseWriter: w, status: http.StatusOK}

		next.ServeHTTP(rw, r)

		ctx := r.Context()

		log.Info().
			Str("request_id", contextx.Get(ctx, contextx.RequestIDKey)).
			Str("user_id", contextx.Get(ctx, contextx.UserIDKey)).
			Str("tenant_id", contextx.Get(ctx, contextx.TenantIDKey)).
			Str("method", r.Method).
			Str("path", r.URL.Path).
			Int("status", rw.status).
			Int64("duration_us", time.Since(start).Microseconds()).
			Msg("http request")
	})
}

func AuthMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		cookie, err := r.Cookie("token")
		if err != nil || cookie.Value == "" {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}

		// TEMP: enquanto não usa JWT
		userID := cookie.Value

		ctx := contextx.Set(r.Context(), contextx.UserIDKey, userID)

		next.ServeHTTP(w, r.WithContext(ctx))
	})
}
