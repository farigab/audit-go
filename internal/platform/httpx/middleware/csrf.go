package middleware

import (
	"crypto/subtle"
	"net/http"
)

// CSRF protects cookie-authenticated mutating requests with a double-submit
// token. It only applies when one of the configured auth cookies is present.
func CSRF(csrfCookieName, csrfHeaderName string, authCookieNames ...string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if !isMutatingMethod(r.Method) || !hasAnyCookie(r, authCookieNames) {
				next.ServeHTTP(w, r)
				return
			}

			cookie, err := r.Cookie(csrfCookieName)
			if err != nil || cookie.Value == "" {
				http.Error(w, "csrf token required", http.StatusForbidden)
				return
			}
			header := r.Header.Get(csrfHeaderName)
			if header == "" || subtle.ConstantTimeCompare([]byte(cookie.Value), []byte(header)) != 1 {
				http.Error(w, "csrf token invalid", http.StatusForbidden)
				return
			}

			next.ServeHTTP(w, r)
		})
	}
}

func isMutatingMethod(method string) bool {
	switch method {
	case http.MethodPost, http.MethodPut, http.MethodPatch, http.MethodDelete:
		return true
	default:
		return false
	}
}

func hasAnyCookie(r *http.Request, names []string) bool {
	for _, name := range names {
		if cookie, err := r.Cookie(name); err == nil && cookie.Value != "" {
			return true
		}
	}
	return false
}
