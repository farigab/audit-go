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

	if cfg.JWTSecret == "" {
		log.Fatal().Msg("JWT_SECRET must not be empty")
	}

	db, err := postgres.Connect(cfg.DBurl)
	if err != nil {
		log.Fatal().Err(err).Msg("failed to connect to postgres")
	}

	// repositories
	docRepo := postgres.NewDocumentRepository(db)
	auditRepo := postgres.NewAuditEventRepository(db)
	userRepo := postgres.NewUserRepository(db)
	refreshRepo := postgres.NewRefreshTokenRepository(db)

	// services
	jwtSvc, err := security.NewJWTService(cfg.JWTSecret, 900)
	if err != nil {
		log.Fatal().Err(err).Msg("failed to create jwt service")
	}

	// use cases
	login := usecase.LoginUseCase{
		UserRepo:    userRepo,
		RefreshRepo: refreshRepo,
		JWT:         jwtSvc,
	}

	logout := usecase.LogoutUseCase{
		RefreshRepo: refreshRepo,
	}

	refresh := usecase.RefreshUseCase{
		UserRepo:    userRepo,
		RefreshRepo: refreshRepo,
		JWT:         jwtSvc,
	}

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
		JWT:            jwtSvc,
		Login:          login,
		Logout:         logout,
		Refresh:        refresh,
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
