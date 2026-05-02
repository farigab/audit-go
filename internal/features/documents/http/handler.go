// Package http exposes document HTTP handlers.
package http

import (
	"encoding/json"
	"errors"
	nethttp "net/http"

	"github.com/rs/zerolog"

	"audit-go/internal/features/access"
	"audit-go/internal/features/documents"
	"audit-go/internal/features/documents/app"
	"audit-go/internal/platform/contextx"
	"audit-go/internal/platform/httpx"
)

// Handler handles document endpoints.
type Handler struct {
	log            zerolog.Logger
	createDocument app.CreateDocumentUseCase
	deleteDocument app.DeleteDocumentUseCase
	getDocument    app.GetDocumentUseCase
}

// NewHandler creates a document handler wired with the provided use cases.
func NewHandler(
	log zerolog.Logger,
	create app.CreateDocumentUseCase,
	del app.DeleteDocumentUseCase,
	get app.GetDocumentUseCase,
) Handler {
	return Handler{
		log:            log,
		createDocument: create,
		deleteDocument: del,
		getDocument:    get,
	}
}

// RegisterRoutes wires authenticated document routes.
func RegisterRoutes(mux *nethttp.ServeMux, auth func(nethttp.Handler) nethttp.Handler, h Handler) {
	mux.Handle("POST /documents", auth(nethttp.HandlerFunc(h.CreateDocument)))
	mux.Handle("GET /documents/get", auth(nethttp.HandlerFunc(h.GetDocument)))
	mux.Handle("DELETE /documents/delete", auth(nethttp.HandlerFunc(h.DeleteDocument)))
}

// CreateDocument handles POST /documents.
func (h Handler) CreateDocument(w nethttp.ResponseWriter, r *nethttp.Request) {
	var body struct {
		JVID       string         `json:"jv_id"`
		Name       string         `json:"name"`
		Type       documents.Type `json:"type"`
		StorageKey string         `json:"storage_key"`
	}

	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		h.writeError(w, r, nethttp.StatusBadRequest, "invalid request body")
		return
	}

	principal, ok := access.PrincipalFromContext(r.Context())
	if !ok {
		h.writeError(w, r, nethttp.StatusUnauthorized, "unauthorized")
		return
	}

	doc, err := h.createDocument.Execute(r.Context(), principal, app.CreateDocumentInput{
		JVID:       body.JVID,
		RequestID:  contextx.Get(r.Context(), contextx.RequestIDKey),
		Name:       body.Name,
		Type:       body.Type,
		StorageKey: body.StorageKey,
	})
	if err != nil {
		h.writeUseCaseError(w, r, err)
		return
	}

	if err = httpx.WriteJSON(w, nethttp.StatusCreated, doc); err != nil {
		h.logWriteError(r, err)
	}
}

// GetDocument handles GET /documents/get.
func (h Handler) GetDocument(w nethttp.ResponseWriter, r *nethttp.Request) {
	documentID := r.URL.Query().Get("id")
	if documentID == "" {
		h.writeError(w, r, nethttp.StatusBadRequest, "id is required")
		return
	}

	principal, ok := access.PrincipalFromContext(r.Context())
	if !ok {
		h.writeError(w, r, nethttp.StatusUnauthorized, "unauthorized")
		return
	}

	doc, err := h.getDocument.Execute(r.Context(), principal, documentID)
	if err != nil {
		h.writeUseCaseError(w, r, err)
		return
	}

	if err = httpx.WriteJSON(w, nethttp.StatusOK, doc); err != nil {
		h.logWriteError(r, err)
	}
}

// DeleteDocument handles DELETE /documents/delete.
func (h Handler) DeleteDocument(w nethttp.ResponseWriter, r *nethttp.Request) {
	documentID := r.URL.Query().Get("id")
	if documentID == "" {
		h.writeError(w, r, nethttp.StatusBadRequest, "id is required")
		return
	}

	principal, ok := access.PrincipalFromContext(r.Context())
	if !ok {
		h.writeError(w, r, nethttp.StatusUnauthorized, "unauthorized")
		return
	}

	if err := h.deleteDocument.Execute(r.Context(), principal, app.DeleteDocumentInput{
		DocumentID: documentID,
		RequestID:  contextx.Get(r.Context(), contextx.RequestIDKey),
	}); err != nil {
		h.writeUseCaseError(w, r, err)
		return
	}

	if err := httpx.WriteJSON(w, nethttp.StatusOK, map[string]string{
		"status": "deleted",
		"id":     documentID,
	}); err != nil {
		h.logWriteError(r, err)
	}
}

func (h Handler) writeUseCaseError(w nethttp.ResponseWriter, r *nethttp.Request, err error) {
	switch {
	case errors.Is(err, app.ErrInvalidInput):
		h.writeError(w, r, nethttp.StatusBadRequest, "invalid document request")
	case errors.Is(err, app.ErrNotFound):
		h.writeError(w, r, nethttp.StatusNotFound, "document not found")
	case errors.Is(err, access.ErrUnauthenticated):
		h.writeError(w, r, nethttp.StatusUnauthorized, "unauthorized")
	case errors.Is(err, access.ErrForbidden):
		h.writeError(w, r, nethttp.StatusForbidden, "forbidden")
	default:
		h.log.Error().Err(err).Str("path", r.URL.Path).Msg("document handler error")
		h.writeError(w, r, nethttp.StatusInternalServerError, "internal server error")
	}
}

func (h Handler) writeError(w nethttp.ResponseWriter, r *nethttp.Request, status int, message string) {
	if err := httpx.WriteError(w, status, message); err != nil {
		h.logWriteError(r, err)
	}
}

func (h Handler) logWriteError(r *nethttp.Request, err error) {
	h.log.Error().
		Err(err).
		Str("path", r.URL.Path).
		Msg("failed to write response")
}
