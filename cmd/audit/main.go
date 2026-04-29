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

	httpdelivery "audit-go/internal/delivery/http"
	"audit-go/internal/delivery/http/middleware"
	"audit-go/internal/infrastructure/postgres"
	"audit-go/internal/platform/config"
	"audit-go/internal/platform/logger"
	"audit-go/internal/platform/security"
	"audit-go/internal/usecase"
	"audit-go/internal/worker"
)

func main() {
	cfg := config.Load()
	cfgc := config.LoadCookieConfig()

	log := logger.NewPrettyWithLevel(cfg.LogLevel)

	if cfg.EntraTenantID == "" || cfg.EntraClientID == "" {
		log.Fatal().Msg("ENTRA_TENANT_ID and ENTRA_CLIENT_ID must not be empty")
	}

	db, err := postgres.Connect(cfg.DBurl)
	if err != nil {
		log.Fatal().Err(err).Msg("failed to connect to postgres")
	}

	// repositories
	docRepo := postgres.NewDocumentRepository(db)
	auditRepo := postgres.NewAuditEventRepository(db)

	// Microsoft Entra ID token validator
	entra, err := security.NewEntraTokenValidator(security.EntraConfig{
		TenantID: cfg.EntraTenantID,
		ClientID: cfg.EntraClientID,
	})
	if err != nil {
		log.Fatal().Err(err).Msg("failed to create Entra token validator")
	}

	// use cases
	createDoc := usecase.CreateDocumentUseCase{
		DocRepo:   docRepo,
		AuditRepo: auditRepo,
	}

	deleteDoc := usecase.DeleteDocumentUseCase{
		DocRepo:   docRepo,
		AuditRepo: auditRepo,
	}

	getDoc := usecase.GetDocumentUseCase{
		DocRepo: docRepo,
	}

	// router
	mux := httpdelivery.RegisterRoutes(httpdelivery.Dependencies{
		Log:            log,
		Config:         cfgc,
		Entra:          entra,
		CreateDocument: createDoc,
		DeleteDocument: deleteDoc,
		GetDocument:    getDoc,
	})

	var app http.Handler = mux
	app = middleware.CORSMiddleware(cfg)(app)
	app = middleware.Logging(log)(app)
	app = middleware.RequestContext(app)

	// context cancelled on SIGTERM / SIGINT — shared with the worker
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGTERM, syscall.SIGINT)
	defer stop()

	// worker
	w := worker.New(log)
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
