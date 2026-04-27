package http

import (
	"net/http"

	"github.com/google/uuid"
	"github.com/rs/zerolog"

	"audit-go/internal/domain"
	"audit-go/internal/platform/contextx"
)

type Handler struct {
	Log zerolog.Logger
}

func NewHandler(log zerolog.Logger) Handler {
	return Handler{Log: log}
}

func (h Handler) Health(w http.ResponseWriter, r *http.Request) {
	if err := WriteText(w, http.StatusOK, "ok"); err != nil {
		h.logWriteError(r, err)
	}
}

func (h Handler) DeleteDocument(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	documentID := r.URL.Query().Get("id")

	if documentID == "" {
		if err := WriteError(w, http.StatusBadRequest, "id is required"); err != nil {
			h.logWriteError(r, err)
		}
		return
	}

	event := domain.NewAuditEvent(
		uuid.NewString(),
		contextx.Get(ctx, contextx.TenantIDKey),
		contextx.Get(ctx, contextx.UserIDKey),
		contextx.Get(ctx, contextx.RequestIDKey),
		domain.ActionDocumentDeleted,
		documentID,
		domain.TargetDocument,
	)

	h.Log.Info().
		Str("event", string(event.Action)).
		Str("target_id", event.TargetID).
		Str("actor_id", event.ActorID).
		Str("request_id", event.RequestID).
		Msg("audit")

	if err := WriteJSON(w, http.StatusOK, map[string]string{"status": "deleted", "id": documentID}); err != nil {
		h.logWriteError(r, err)
	}
}

func (h Handler) logWriteError(r *http.Request, err error) {
	h.Log.Error().
		Err(err).
		Str("path", r.URL.Path).
		Msg("failed to write response")
}
