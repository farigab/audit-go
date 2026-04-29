package middleware

import (
	"net/http"

	"audit-go/internal/platform/config"
	"audit-go/internal/platform/origin"
)

// CORSMiddleware returns a middleware that sets CORS headers based on config.
// A Allowlist é construída uma vez no startup — zero alocações por request.
func CORSMiddleware(cfg *config.Config) func(http.Handler) http.Handler {
	var al origin.Allowlist
	if cfg != nil {
		al = origin.Parse(cfg.AllowedOrigins)
	}

	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if handled := applyCORS(w, r, al); handled {
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}

// applyCORS aplica os headers CORS e reporta se o request foi encerrado
// (preflight respondido ou origin bloqueada). Retorna false quando o
// handler principal deve continuar normalmente.
func applyCORS(w http.ResponseWriter, r *http.Request, al origin.Allowlist) bool {
	originHeader := r.Header.Get("Origin")

	if originHeader == "" {
		setAllowHeaders(w, r)
		return false
	}

	if !al.Allows(originHeader) {
		return rejectOrigin(w, r)
	}

	setOriginHeaders(w, originHeader)
	setAllowHeaders(w, r)

	if r.Method == http.MethodOptions {
		w.WriteHeader(http.StatusNoContent)
		return true
	}

	return false
}

func rejectOrigin(w http.ResponseWriter, r *http.Request) bool {
    if r.Method == http.MethodOptions {
        http.Error(w, "origin not allowed", http.StatusForbidden)
        return true
    }

    return false
}

func setOriginHeaders(w http.ResponseWriter, originHeader string) {
	w.Header().Set("Access-Control-Allow-Origin", originHeader)
	w.Header().Set("Vary", "Origin")
	w.Header().Set("Access-Control-Allow-Credentials", "true")
}

func setAllowHeaders(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, PATCH, DELETE, OPTIONS")
	if reqHeaders := r.Header.Get("Access-Control-Request-Headers"); reqHeaders != "" {
		w.Header().Set("Access-Control-Allow-Headers", reqHeaders)
	} else {
		w.Header().Set("Access-Control-Allow-Headers", "Accept, Content-Type, Authorization, X-Requested-With, X-Health-Check")
	}
}
