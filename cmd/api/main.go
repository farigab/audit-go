package main

import (
	"net/http"

	httpdelivery "audit-go/internal/delivery/http"
	"audit-go/internal/platform/logger"
)

func main() {
	log := logger.NewPretty()

	handler := httpdelivery.NewHandler(log)

	mux := http.NewServeMux()

	mux.HandleFunc("/health", handler.Health)
	mux.HandleFunc("/documents/delete", handler.DeleteDocument)

	var app http.Handler = mux
	app = httpdelivery.RequestContext(log, app)
	app = httpdelivery.Logging(log, app)

	log.Info().Msg("server started :8080")

	if err := http.ListenAndServe(":8080", app); err != nil {
		log.Fatal().Err(err).Msg("server failed")
	}
}
