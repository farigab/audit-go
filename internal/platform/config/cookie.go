// Package config handles loading application configuration from environment.
package config

import (
	"os"

	"github.com/joho/godotenv"
)

// Config stores environment variables used by the application.
type CookieConfig struct {
	CookieSecure   bool
	CookieSameSite string
	CookieDomain   string
}

// Load reads environment variables (.env optional) and returns the config.
func LoadCookieConfig() *CookieConfig {
	_ = godotenv.Load()

	cookieSecure := defaultBool(os.Getenv("COOKIE_SECURE"), true)
	cookieSameSite := defaultString(os.Getenv("COOKIE_SAME_SITE"), "Lax")
	cookieDomain := defaultString(os.Getenv("COOKIE_DOMAIN"), "")

	return &CookieConfig{
		CookieSecure:   cookieSecure,
		CookieSameSite: cookieSameSite,
		CookieDomain:   cookieDomain,
	}
}
