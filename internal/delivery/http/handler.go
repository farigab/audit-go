package http

import (
	"net/http"

	"github.com/rs/zerolog"
)

type Handler struct {
	Log zerolog.Logger
}

func NewHandler(log zerolog.Logger) Handler {
	return Handler{Log: log}
}

func (h Handler) Health(w http.ResponseWriter, r *http.Request) {
	if err := WriteText(w, http.StatusOK, "ok"); err != nil {
		h.Log.Error().
			Err(err).
			Msg("failed to write health response")
	}
}

func (h Handler) DeleteDocument(w http.ResponseWriter, r *http.Request) {
	documentID := r.URL.Query().Get("id")

	h.Log.Info().
		Str("event", "document_deleted").
		Str("document_id", documentID).
		Msg("audit log")

	if err := WriteJSON(w, http.StatusOK, map[string]string{
		"status": "deleted",
		"id":     documentID,
	}); err != nil {
		h.Log.Error().
			Err(err).
			Msg("failed to write delete response")
	}
}
