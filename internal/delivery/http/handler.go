package http

import (
	"net/http"

	"github.com/rs/zerolog"

	"audit-go/internal/platform/contextx"
	"audit-go/internal/usecase"
)

type Handler struct {
	Log    zerolog.Logger
	delete usecase.DeleteDocumentUseCase
}

func NewHandler(log zerolog.Logger, delete usecase.DeleteDocumentUseCase) Handler {
	return Handler{Log: log, delete: delete}
}

func (h Handler) Health(w http.ResponseWriter, r *http.Request) {
	if err := WriteText(w, http.StatusOK, "ok"); err != nil {
		h.logWriteError(r, err)
	}
}

func (h Handler) DeleteDocument(w http.ResponseWriter, r *http.Request) {
	documentID := r.URL.Query().Get("id")
	if documentID == "" {
		if err := WriteError(w, http.StatusBadRequest, "id is required"); err != nil {
			h.logWriteError(r, err)
		}
		return
	}

	ctx := r.Context()
	err := h.delete.Execute(usecase.DeleteDocumentInput{
		DocumentID: documentID,
		ActorID:    contextx.Get(ctx, contextx.UserIDKey),
		TenantID:   contextx.Get(ctx, contextx.TenantIDKey),
		RequestID:  contextx.Get(ctx, contextx.RequestIDKey),
	})
	if err != nil {
		if werr := WriteError(w, http.StatusNotFound, err.Error()); werr != nil {
			h.logWriteError(r, werr)
		}
		return
	}

	if err := WriteJSON(w, http.StatusOK, map[string]string{
		"status": "deleted",
		"id":     documentID,
	}); err != nil {
		h.logWriteError(r, err)
	}
}

func (h Handler) logWriteError(r *http.Request, err error) {
	h.Log.Error().
		Err(err).
		Str("path", r.URL.Path).
		Msg("failed to write response")
}
