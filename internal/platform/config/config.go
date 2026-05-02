// Package config handles loading application configuration from environment.
package config

import (
	"os"
	"time"

	"github.com/joho/godotenv"
	"github.com/rs/zerolog"
)

// Config stores environment variables used by the application.
type Config struct {
	DBurl            string
	LogLevel         zerolog.Level
	PythonServiceURL string
	Port             string
	AllowedOrigins   string

	// Microsoft Entra ID (Azure AD)
	EntraTenantID     string
	EntraClientID     string
	EntraClientSecret string
	EntraRedirectURL  string

	AuthSuccessRedirectURL string
	SessionTTL             time.Duration
	RefreshTTL             time.Duration
}

// Load reads environment variables (.env optional) and returns the config.
func Load() *Config {
	_ = godotenv.Load()

	dbURL := defaultString(os.Getenv("DB_URL"), "postgres://audit:audit@localhost:5432/auditdb?sslmode=disable")

	logLevelStr := defaultString(os.Getenv("LOG_LEVEL"), "info")
	lvl, err := zerolog.ParseLevel(logLevelStr)
	if err != nil {
		lvl = zerolog.InfoLevel
	}

	return &Config{
		DBurl:             dbURL,
		LogLevel:          lvl,
		PythonServiceURL:  defaultString(os.Getenv("PYTHON_SERVICE_URL"), "http://localhost:8000"),
		Port:              defaultString(os.Getenv("PORT"), ":8080"),
		AllowedOrigins:    defaultString(os.Getenv("ALLOWED_ORIGINS"), ""),
		EntraTenantID:     os.Getenv("ENTRA_TENANT_ID"),
		EntraClientID:     os.Getenv("ENTRA_CLIENT_ID"),
		EntraClientSecret: os.Getenv("ENTRA_CLIENT_SECRET"),
		EntraRedirectURL:  defaultString(os.Getenv("ENTRA_REDIRECT_URL"), "http://localhost:8080/auth/callback"),

		AuthSuccessRedirectURL: defaultString(os.Getenv("AUTH_SUCCESS_REDIRECT_URL"), "http://localhost:3000"),
		SessionTTL:             defaultDuration(os.Getenv("APP_SESSION_TTL"), 15*time.Minute),
		RefreshTTL:             defaultDuration(os.Getenv("APP_REFRESH_TTL"), 30*24*time.Hour),
	}
}
