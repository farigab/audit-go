package main

import (
	"net/http"
	"os"

	httpdelivery "audit-go/internal/delivery/http"
	"audit-go/internal/infrastructure/memory"
	"audit-go/internal/platform/logger"
	"audit-go/internal/usecase"
	"audit-go/internal/worker"
)

func main() {
	log := logger.NewPretty()

	docRepo := memory.NewDocumentRepository()
	auditRepo := memory.NewAuditEventRepository()

	handler := httpdelivery.NewHandler(
		log,
		usecase.CreateDocumentUseCase{DocRepo: docRepo, AuditRepo: auditRepo},
		usecase.DeleteDocumentUseCase{DocRepo: docRepo, AuditRepo: auditRepo},
		usecase.GetDocumentUseCase{DocRepo: docRepo},
	)

	mux := http.NewServeMux()
	mux.HandleFunc("/health", handler.Health)
	mux.HandleFunc("/documents", handler.CreateDocument)        // POST
	mux.HandleFunc("/documents/get", handler.GetDocument)       // GET  ?id=
	mux.HandleFunc("/documents/delete", handler.DeleteDocument) // DELETE ?id=

	var app http.Handler = mux
	app = httpdelivery.RequestContext(app)
	app = httpdelivery.Logging(log, app)

	w := worker.New(log)
	go w.Start()

	addr := envOr("ADDR", ":8080")
	log.Info().Str("addr", addr).Msg("server started")

	if err := http.ListenAndServe(addr, app); err != nil {
		log.Fatal().Err(err).Msg("server failed")
	}
}

func envOr(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}
