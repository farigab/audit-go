// Package config handles loading application configuration from environment.
package config

import (
	"os"

	"github.com/joho/godotenv"
	"github.com/rs/zerolog"
)

// Config stores environment variables used by the application.
type Config struct {
	DBurl            string
	LogLevel         zerolog.Level
	PythonServiceURL string
	Port             string
	AllowedOrigins    string
	JWTSecret        string
}

// Load reads environment variables (.env optional) and returns the config.
func Load() *Config {
	_ = godotenv.Load()

	PostGreeURL := defaultString(os.Getenv("DB_URL"), "postgres://audit:audit@localhost:5432/auditdb?sslmode=disable")
	logLevelStr := defaultString(os.Getenv("LOG_LEVEL"), "info")
	lvl, err := zerolog.ParseLevel(logLevelStr)
	if err != nil {
		lvl = zerolog.InfoLevel
	}
	PythonServiceURL := defaultString(os.Getenv("PYTHON_SERVICE_URL"), "http://localhost:8000")
	PORT := defaultString(os.Getenv("PORT"), ":8080")
	AllowedOrigins := defaultString(os.Getenv("ALLOWED_ORIGINS"), "")


	return &Config{
		DBurl:            PostGreeURL,
		LogLevel:         lvl,
		PythonServiceURL: PythonServiceURL,
		Port:             PORT,
		AllowedOrigins:   AllowedOrigins,
		JWTSecret:        os.Getenv("JWT_SECRET"),
	}
}
