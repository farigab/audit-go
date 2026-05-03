package middleware

import (
	"net/http"
	"time"

	"github.com/rs/zerolog"

	"audit-go/internal/platform/contextx"
)

type responseWriter struct {
	http.ResponseWriter
	status      int
	bytes       int
	wroteHeader bool
}

func newResponseWriter(w http.ResponseWriter) *responseWriter {
	return &responseWriter{
		ResponseWriter: w,
		status:         http.StatusOK,
	}
}

func (rw *responseWriter) WriteHeader(status int) {
	if rw.wroteHeader {
		return
	}
	rw.status = status
	rw.wroteHeader = true
	rw.ResponseWriter.WriteHeader(status)
}

func (rw *responseWriter) Write(b []byte) (int, error) {
	if !rw.wroteHeader {
		rw.WriteHeader(http.StatusOK)
	}
	n, err := rw.ResponseWriter.Write(b)
	rw.bytes += n
	return n, err
}

// Logging logs HTTP requests with latency, status and request context values.
func Logging(log zerolog.Logger) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			start := time.Now()
			rw := newResponseWriter(w)

			next.ServeHTTP(rw, r)

			ctx := r.Context()

			log.Info().
				Str("request_id", contextx.Get(ctx, contextx.RequestIDKey)).
				Str("user_id", contextx.Get(ctx, contextx.UserIDKey)).
				Str("method", r.Method).
				Str("path", r.URL.Path).
				Str("query", r.URL.RawQuery).
				Str("remote_addr", r.RemoteAddr).
				Str("user_agent", r.UserAgent()).
				Int("status", rw.status).
				Int("bytes", rw.bytes).
				Int64("duration_ms", time.Since(start).Milliseconds()).
				Msg("http request")
		})
	}
}
