// Package main is the audit API server entrypoint.
package main

import (
	"net/http"

	"audit-go/internal/config"
	httpdelivery "audit-go/internal/delivery/http"
	"audit-go/internal/infrastructure/memory"
	"audit-go/internal/infrastructure/postgres"
	"audit-go/internal/platform/logger"
	"audit-go/internal/usecase"
	"audit-go/internal/worker"
)

func main() {
	cfg := config.Load()
	log := logger.NewPrettyWithLevel(cfg.LogLevel)

	dsn := cfg.DBurl

	db, err := postgres.Connect(dsn)
	if err != nil {
		log.Fatal().Err(err).Msg("failed to connect to postgres")
	}

	docRepo := postgres.NewDocumentRepository(db)
	auditRepo := postgres.NewAuditEventRepository(db)

	handler := httpdelivery.NewHandler(
		log,
		usecase.CreateDocumentUseCase{DocRepo: docRepo, AuditRepo: auditRepo},
		usecase.DeleteDocumentUseCase{DocRepo: docRepo, AuditRepo: auditRepo},
		usecase.GetDocumentUseCase{DocRepo: docRepo},
	)

	mux := http.NewServeMux()

	// register auth routes (minimal)
	refreshRepo := memory.NewRefreshTokenRepo()
	httpdelivery.RegisterAuthRoutes(mux, refreshRepo)

	mux.HandleFunc("/health", handler.Health)
	mux.HandleFunc("/documents", handler.CreateDocument)        // POST
	mux.HandleFunc("/documents/get", handler.GetDocument)       // GET  ?id=
	mux.HandleFunc("/documents/delete", handler.DeleteDocument) // DELETE ?id=

	var app http.Handler = mux
	app = httpdelivery.RequestContext(app)
	app = httpdelivery.Logging(log, app)

	w := worker.New(log)
	go w.Start()

	addr := cfg.Port
	log.Info().Str("addr", addr).Msg("server started")

	if err := http.ListenAndServe(addr, app); err != nil {
		log.Fatal().Err(err).Msg("server failed")
	}
}
