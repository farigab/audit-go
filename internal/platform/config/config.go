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
	DBurl             string
	DBMaxOpenConns    int
	DBMaxIdleConns    int
	DBConnMaxLifetime time.Duration
	DBConnMaxIdleTime time.Duration
	LogLevel          zerolog.Level
	PythonServiceURL  string
	Port              string
	AllowedOrigins    string
	TrustProxy        bool
	UploadURLTTL      time.Duration

	AzureStorageAccountName string
	AzureStorageContainer   string
	AzureStorageEndpoint    string

	// Microsoft Entra ID (Azure AD)
	EntraTenantID     string
	EntraClientID     string
	EntraClientSecret string
	EntraRedirectURL  string

	AuthSuccessRedirectURL string
	SessionTTL             time.Duration
	RefreshTTL             time.Duration
	AuthCleanupInterval    time.Duration
	PrincipalCacheTTL      time.Duration
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
		DBMaxOpenConns:    defaultInt(os.Getenv("DB_MAX_OPEN_CONNS"), 25),
		DBMaxIdleConns:    defaultInt(os.Getenv("DB_MAX_IDLE_CONNS"), 5),
		DBConnMaxLifetime: defaultDuration(os.Getenv("DB_CONN_MAX_LIFETIME"), 5*time.Minute),
		DBConnMaxIdleTime: defaultDuration(os.Getenv("DB_CONN_MAX_IDLE_TIME"), 2*time.Minute),
		LogLevel:          lvl,
		PythonServiceURL:  defaultString(os.Getenv("PYTHON_SERVICE_URL"), "http://localhost:8000"),
		Port:              defaultString(os.Getenv("PORT"), ":8080"),
		AllowedOrigins:    defaultString(os.Getenv("ALLOWED_ORIGINS"), ""),
		TrustProxy:        defaultBool(os.Getenv("TRUST_PROXY"), false),
		UploadURLTTL:      defaultDuration(os.Getenv("DOCUMENT_UPLOAD_URL_TTL"), 15*time.Minute),

		AzureStorageAccountName: os.Getenv("AZURE_STORAGE_ACCOUNT_NAME"),
		AzureStorageContainer:   defaultString(os.Getenv("AZURE_STORAGE_BLOB_CONTAINER"), "documents"),
		AzureStorageEndpoint:    os.Getenv("AZURE_STORAGE_ENDPOINT"),
		EntraTenantID:           os.Getenv("ENTRA_TENANT_ID"),
		EntraClientID:           os.Getenv("ENTRA_CLIENT_ID"),
		EntraClientSecret:       os.Getenv("ENTRA_CLIENT_SECRET"),
		EntraRedirectURL:        defaultString(os.Getenv("ENTRA_REDIRECT_URL"), "http://localhost:8080/auth/callback"),

		AuthSuccessRedirectURL: defaultString(os.Getenv("AUTH_SUCCESS_REDIRECT_URL"), "http://localhost:5173"),
		SessionTTL:             defaultDuration(os.Getenv("APP_SESSION_TTL"), 15*time.Minute),
		RefreshTTL:             defaultDuration(os.Getenv("APP_REFRESH_TTL"), 30*24*time.Hour),
		AuthCleanupInterval:    defaultDuration(os.Getenv("AUTH_CLEANUP_INTERVAL"), 10*time.Minute),
		PrincipalCacheTTL:      defaultDuration(os.Getenv("ACCESS_PRINCIPAL_CACHE_TTL"), 30*time.Second),
	}
}
