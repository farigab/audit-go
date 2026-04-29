// Package cookies provides shared HTTP cookie helpers used across handlers and middleware.
package cookies

import (
	"net/http"

	"audit-go/internal/platform/config"
)

// SetWithPath writes an HttpOnly cookie with a custom Path and settings
// derived from cfg. Use maxAge=-1 to delete the cookie.
func SetWithPath(w http.ResponseWriter, name, value string, maxAge int, cfg *config.CookieConfig, path string) {
	secure := true
	sameSite := http.SameSiteLaxMode
	var domain string
	if cfg != nil {
		secure = cfg.CookieSecure
		sameSite = parseSameSite(cfg.CookieSameSite)
		domain = cfg.CookieDomain
	}

	c := &http.Cookie{
		Name:     name,
		Value:    value,
		HttpOnly: true,
		Secure:   secure,
		Path:     path,
		MaxAge:   maxAge,
		SameSite: sameSite,
	}

	// Setting Domain for localhost breaks cookies in most browsers.
	if domain != "" && domain != "localhost" {
		c.Domain = domain
	}

	http.SetCookie(w, c)
}

// ClearAuth deletes the token and refreshToken cookies.
func ClearAuth(w http.ResponseWriter, cfg *config.CookieConfig) {
	SetWithPath(w, "token", "", -1, cfg, "/")
	SetWithPath(w, "refreshToken", "", -1, cfg, "/auth/refresh")
}

// parseSameSite converts the string config value to http.SameSite.
func parseSameSite(s string) http.SameSite {
	switch s {
	case "Strict":
		return http.SameSiteStrictMode
	case "None":
		return http.SameSiteNoneMode
	default:
		return http.SameSiteLaxMode
	}
}
