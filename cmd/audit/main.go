// Package main is the audit API server entrypoint.
package main

import (
	"net/http"

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
		UserRepo:       userRepo,
		RefreshRepo:    refreshRepo,
		Login:          login,
		Logout:         logout,
		CreateDocument: createDoc,
		DeleteDocument: deleteDoc,
		GetDocument:    getDoc,
	})

	// middleware chain
	var app http.Handler = mux
	app = middleware.RequestContext(app)
	app = middleware.Logging(log)(app)

	// worker
	w := worker.New(log)
	go w.Start()

	log.Info().
		Str("addr", cfg.Port).
		Msg("server started")

	if err = http.ListenAndServe(cfg.Port, app); err != nil {
		log.Fatal().
			Err(err).
			Msg("server failed")
	}
}
