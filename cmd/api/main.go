package main

import (
	"net/http"
	"os"

	httpdelivery "audit-go/internal/delivery/http"
	"audit-go/internal/infrastructure/memory"
	"audit-go/internal/platform/logger"
	"audit-go/internal/usecase"
)

func main() {
	log := logger.NewPretty()

	// repositórios em memória — trocar por postgres quando o banco estiver pronto
	docRepo := memory.NewDocumentRepository()
	auditRepo := memory.NewAuditEventRepository()

	// usecases
	deleteDoc := usecase.DeleteDocumentUseCase{
		DocRepo:   docRepo,
		AuditRepo: auditRepo,
	}

	// handler com dependências injetadas
	handler := httpdelivery.NewHandler(log, deleteDoc)

	// rotas
	mux := http.NewServeMux()
	mux.HandleFunc("/health", handler.Health)
	mux.HandleFunc("/documents/delete", handler.DeleteDocument)

	// middlewares — ordem importa: RequestContext antes do Logging
	// para que o Logging já enxergue request_id, user_id, tenant_id
	var app http.Handler = mux
	app = httpdelivery.RequestContext(log, app)
	app = httpdelivery.Logging(log, app)

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
