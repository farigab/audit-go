package config

import (
	"os"
	"strings"

	"github.com/joho/godotenv"
	"github.com/rs/zerolog"
)

type CookieConfig struct {
	Name     string
	Domain   string
	Path     string
	HttpOnly bool
	Secure   bool
	SameSite string
}

type Config struct {
	DBurl            string
	LogLevel         zerolog.Level
	PythonServiceURL string
	Port             string
	Cookie           CookieConfig
}

func Load() *Config {
	_ = godotenv.Load()

	dbURL := defaultString(os.Getenv("DB_URL"), "postgres://audit:audit@localhost:5432/auditdb?sslmode=disable")

	logLevelStr := defaultString(os.Getenv("LOG_LEVEL"), "info")
	lvl, err := zerolog.ParseLevel(logLevelStr)
	if err != nil {
		lvl = zerolog.InfoLevel
	}

	pythonServiceURL := defaultString(os.Getenv("PYTHON_SERVICE_URL"), "http://localhost:8000")
	port := defaultString(os.Getenv("PORT"), ":8080")

	cookie := CookieConfig{
		Name:     defaultString(os.Getenv("COOKIE_NAME"), "session_id"),
		Domain:   defaultString(os.Getenv("COOKIE_DOMAIN"), "localhost"),
		Path:     defaultString(os.Getenv("COOKIE_PATH"), "/"),
		HttpOnly: getEnvBool("COOKIE_HTTP_ONLY", true),
		Secure:   getEnvBool("COOKIE_SECURE", false),
		SameSite: defaultString(os.Getenv("COOKIE_SAMESITE"), "Lax"),
	}

	return &Config{
		DBurl:            dbURL,
		LogLevel:         lvl,
		PythonServiceURL: pythonServiceURL,
		Port:             port,
		Cookie:           cookie,
	}
}

func defaultString(v, d string) string {
	if v == "" {
		return d
	}
	return v
}

func getEnvBool(key string, def bool) bool {
	v := strings.ToLower(os.Getenv(key))
	if v == "true" || v == "1" {
		return true
	}
	if v == "false" || v == "0" {
		return false
	}
	return def
}
