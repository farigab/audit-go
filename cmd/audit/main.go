// Package main is the audit API server entrypoint.
package main

import (
	"context"
	"errors"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	accessapp "audit-go/internal/features/access/app"
	accesshttp "audit-go/internal/features/access/http"
	accesspostgres "audit-go/internal/features/access/postgres"
	auditpostgres "audit-go/internal/features/audit/postgres"
	documentsapp "audit-go/internal/features/documents/app"
	documentshttp "audit-go/internal/features/documents/http"
	documentpostgres "audit-go/internal/features/documents/postgres"
	processingworker "audit-go/internal/features/processing/worker"
	"audit-go/internal/platform/config"
	"audit-go/internal/platform/httpx"
	"audit-go/internal/platform/httpx/middleware"
	"audit-go/internal/platform/logger"
	platformpostgres "audit-go/internal/platform/postgres"
	"audit-go/internal/platform/security"
)

func main() {
	cfg := config.Load()
	cookieCfg := config.LoadCookieConfig()

	log := logger.NewPrettyWithLevel(cfg.LogLevel)

	if cfg.EntraTenantID == "" || cfg.EntraClientID == "" {
		log.Fatal().Msg("ENTRA_TENANT_ID and ENTRA_CLIENT_ID must not be empty")
	}

	db, err := platformpostgres.Connect(cfg.DBurl)
	if err != nil {
		log.Fatal().Err(err).Msg("failed to connect to postgres")
	}

	// repositories
	docRepo := documentpostgres.NewRepository(db)
	auditRepo := auditpostgres.NewRepository(db)
	authorizer := accesspostgres.NewAuthorizer(db)
	sessionRepo := accesspostgres.NewSessionRepository(db)
	transactor := platformpostgres.NewTransactor(db)

	// Microsoft Entra ID token validator
	entra, err := security.NewEntraTokenValidator(security.EntraConfig{
		TenantID: cfg.EntraTenantID,
		ClientID: cfg.EntraClientID,
	})
	if err != nil {
		log.Fatal().Err(err).Msg("failed to create Entra token validator")
	}

	authService := accessapp.NewService(accessapp.Config{
		TenantID:           cfg.EntraTenantID,
		ClientID:           cfg.EntraClientID,
		ClientSecret:       cfg.EntraClientSecret,
		RedirectURL:        cfg.EntraRedirectURL,
		SuccessRedirectURL: cfg.AuthSuccessRedirectURL,
		AllowedOrigins:     cfg.AllowedOrigins,
		SessionTTL:         cfg.SessionTTL,
		RefreshTTL:         cfg.RefreshTTL,
	}, sessionRepo, entra)

	// use cases
	createDoc := documentsapp.CreateDocumentUseCase{
		DocRepo:    docRepo,
		AuditRepo:  auditRepo,
		Authorizer: authorizer,
		Transactor: transactor,
	}

	deleteDoc := documentsapp.DeleteDocumentUseCase{
		DocRepo:    docRepo,
		AuditRepo:  auditRepo,
		Authorizer: authorizer,
		Transactor: transactor,
	}

	getDoc := documentsapp.GetDocumentUseCase{
		DocRepo:    docRepo,
		Authorizer: authorizer,
	}

	// router
	mux := http.NewServeMux()
	mux.HandleFunc("GET /health", func(w http.ResponseWriter, r *http.Request) {
		if err := httpx.WriteText(w, http.StatusOK, "ok"); err != nil {
			log.Error().Err(err).Str("path", r.URL.Path).Msg("failed to write response")
		}
	})

	auth := middleware.Auth(entra, authService, accesshttp.SessionCookie)
	accesshttp.RegisterRoutes(
		mux,
		auth,
		accesshttp.NewHandler(log, authService, cookieCfg),
	)
	documentshttp.RegisterRoutes(
		mux,
		auth,
		documentshttp.NewHandler(log, createDoc, deleteDoc, getDoc),
	)

	var app http.Handler = mux
	app = middleware.CSRF(accesshttp.CSRFCookie, accesshttp.CSRFHeader, accesshttp.SessionCookie, accesshttp.RefreshCookie)(app)
	app = middleware.CORSMiddleware(cfg)(app)
	app = middleware.Logging(log)(app)
	app = middleware.RequestContext(app)

	// context cancelled on SIGTERM / SIGINT — shared with the worker
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGTERM, syscall.SIGINT)
	defer stop()

	// worker
	w := processingworker.New(log)
	go w.Start(ctx)

	srv := &http.Server{
		Addr:              cfg.Port,
		Handler:           app,
		ReadTimeout:       15 * time.Second,
		ReadHeaderTimeout: 5 * time.Second,
		WriteTimeout:      30 * time.Second,
		IdleTimeout:       120 * time.Second,
	}

	go func() {
		log.Info().Str("addr", cfg.Port).Msg("server started")
		if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			log.Fatal().Err(err).Msg("server failed")
		}
	}()

	<-ctx.Done()
	stop()

	log.Info().Msg("shutting down server")

	shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	if err := srv.Shutdown(shutdownCtx); err != nil {
		log.Error().Err(err).Msg("server shutdown error")
		os.Exit(1)
	}

	log.Info().Msg("server stopped cleanly")
}
