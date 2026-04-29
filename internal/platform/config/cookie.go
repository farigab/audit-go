// Package config handles loading application configuration from environment.
package config

import (
	"os"

	"github.com/joho/godotenv"
)

// CookieConfig stores cookie-related configuration and the CORS allowed origins
// list, grouped here to avoid passing the full Config into auth middleware.
type CookieConfig struct {
	CookieSecure   bool
	CookieSameSite string
	CookieDomain   string
	AllowedOrigins string
}

// LoadCookieConfig reads cookie/CORS configuration from environment variables.
// Env var names match .env.example exactly (APP_COOKIE_* prefix).
func LoadCookieConfig() *CookieConfig {
	_ = godotenv.Load()

	return &CookieConfig{
		CookieSecure:   defaultBool(os.Getenv("APP_COOKIE_SECURE"), true),
		CookieSameSite: defaultString(os.Getenv("APP_COOKIE_SAME_SITE"), "Lax"),
		CookieDomain:   defaultString(os.Getenv("APP_COOKIE_DOMAIN"), ""),
		AllowedOrigins: os.Getenv("ALLOWED_ORIGINS"),
	}
}
