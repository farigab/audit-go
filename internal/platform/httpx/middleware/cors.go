package middleware

import (
	"net/http"

	"audit-go/internal/platform/config"
	"audit-go/internal/platform/origin"
)

// CORSMiddleware returns a middleware that enforces the CORS policy defined in cfg.
func CORSMiddleware(cfg *config.Config) func(http.Handler) http.Handler {
	var al origin.Allowlist
	if cfg != nil {
		al = origin.Parse(cfg.AllowedOrigins)
	}

	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			originHeader := r.Header.Get("Origin")

			if originHeader == "" {
				next.ServeHTTP(w, r)
				return
			}

			if !al.Allows(originHeader) {
				http.Error(w, "origin not allowed", http.StatusForbidden)
				return
			}

			setOriginHeaders(w, originHeader)
			setAllowHeaders(w, r)

			if r.Method == http.MethodOptions {
				w.WriteHeader(http.StatusNoContent)
				return
			}

			next.ServeHTTP(w, r)
		})
	}
}

func setOriginHeaders(w http.ResponseWriter, originHeader string) {
	w.Header().Set("Access-Control-Allow-Origin", originHeader)
	w.Header().Set("Vary", "Origin")
	w.Header().Set("Access-Control-Allow-Credentials", "true")
}

func setAllowHeaders(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, PATCH, DELETE, OPTIONS")
	if req := r.Header.Get("Access-Control-Request-Headers"); req != "" {
		w.Header().Set("Access-Control-Allow-Headers", req)
		return
	}
	w.Header().Set("Access-Control-Allow-Headers", "Accept, Content-Type, Authorization, X-Requested-With, X-CSRF-Token")
}
