package middleware

import (
	"net/http"

	"audit-go/internal/platform/config"
	"audit-go/internal/platform/origin"
)

// CORSMiddleware returns a middleware that enforces the CORS policy defined in cfg.
func CORSMiddleware(cfg *config.Config) func(http.Handler) http.Handler {
	// Build the allowlist once at startup — zero allocations per request.
	var al origin.Allowlist
	if cfg != nil {
		al = origin.Parse(cfg.AllowedOrigins)
	}

	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			originHeader := r.Header.Get("Origin")

			// No Origin → non-browser client, CORS policy does not apply.
			if originHeader == "" {
				next.ServeHTTP(w, r)
				return
			}

			if !al.Allows(originHeader) {
				http.Error(w, "origin not allowed", http.StatusForbidden)
				return
			}

			// Origin is allowed: set response headers.
			setOriginHeaders(w, originHeader)
			setAllowHeaders(w, r)

			// Preflight: respond immediately without forwarding to the handler.
			if r.Method == http.MethodOptions {
				w.WriteHeader(http.StatusNoContent)
				return
			}

			next.ServeHTTP(w, r)
		})
	}
}

// setOriginHeaders writes the headers that tell the browser this origin is
// permitted and that credentials (cookies, Authorization) may be included.
func setOriginHeaders(w http.ResponseWriter, originHeader string) {
	// Echo the exact origin instead of "*": browsers reject "*" when
	// credentials are involved (withCredentials / HttpOnly cookies).
	w.Header().Set("Access-Control-Allow-Origin", originHeader)
	// Vary: Origin instructs caches not to serve a cached response for
	// one origin to a different origin.
	w.Header().Set("Vary", "Origin")
	w.Header().Set("Access-Control-Allow-Credentials", "true")
}

// setAllowHeaders writes the headers that describe which methods and request
// headers are permitted. Called for both preflight and actual requests so that
// proxies and CDNs that inspect these headers on non-OPTIONS responses work
// correctly.
func setAllowHeaders(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, PATCH, DELETE, OPTIONS")

	// Mirror the requested headers back rather than maintaining a static
	// allowlist here. The origin allowlist is the security boundary; once an
	// origin is trusted, its headers are trusted too.
	if req := r.Header.Get("Access-Control-Request-Headers"); req != "" {
		w.Header().Set("Access-Control-Allow-Headers", req)
	} else {
		w.Header().Set("Access-Control-Allow-Headers",
			"Accept, Content-Type, Authorization, X-Requested-With, X-Health-Check")
	}
}
