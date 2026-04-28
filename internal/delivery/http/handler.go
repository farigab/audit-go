// Package http contains delivery layer HTTP handlers and helpers.
package http

import (
	"encoding/json"
	"net/http"

	"github.com/rs/zerolog"

	"audit-go/internal/domain"
	"audit-go/internal/platform/contextx"
	"audit-go/internal/usecase"
)

// Handler handles HTTP endpoints for the service.
type Handler struct {
	Log            zerolog.Logger
	createDocument usecase.CreateDocumentUseCase
	deleteDocument usecase.DeleteDocumentUseCase
	getDocument    usecase.GetDocumentUseCase
}

// NewHandler creates a new HTTP Handler wired with the provided use cases.
func NewHandler(
	log zerolog.Logger,
	create usecase.CreateDocumentUseCase,
	del usecase.DeleteDocumentUseCase,
	get usecase.GetDocumentUseCase,
) Handler {
	return Handler{
		Log:            log,
		createDocument: create,
		deleteDocument: del,
		getDocument:    get,
	}
}

const methodNotAllowed = "method not allowed"

// Health responds with a simple liveness check.
func (h Handler) Health(w http.ResponseWriter, r *http.Request) {
	if err := WriteText(w, http.StatusOK, "ok"); err != nil {
		h.logWriteError(r, err)
	}
}

// CreateDocument handles POST /documents to create a new document.
func (h Handler) CreateDocument(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		if err := WriteError(w, http.StatusMethodNotAllowed, methodNotAllowed); err != nil {
			h.logWriteError(r, err)
		}
		return
	}

	var body struct {
		JVID       string         `json:"jv_id"`
		Name       string         `json:"name"`
		Type       domain.DocType `json:"type"`
		StorageKey string         `json:"storage_key"`
	}

	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		if werr := WriteError(w, http.StatusBadRequest, "invalid request body"); werr != nil {
			h.logWriteError(r, werr)
		}
		return
	}

	if body.JVID == "" || body.Name == "" || body.StorageKey == "" {
		if err := WriteError(w, http.StatusBadRequest, "jv_id, name and storage_key are required"); err != nil {
			h.logWriteError(r, err)
		}
		return
	}

	ctx := r.Context()
	doc, err := h.createDocument.Execute(ctx, usecase.CreateDocumentInput{
		JVID:       body.JVID,
		ActorID:    contextx.Get(ctx, contextx.UserIDKey),
		RequestID:  contextx.Get(ctx, contextx.RequestIDKey),
		Name:       body.Name,
		Type:       body.Type,
		StorageKey: body.StorageKey,
	})
	if err != nil {
		if werr := WriteError(w, http.StatusInternalServerError, "failed to create document"); werr != nil {
			h.logWriteError(r, werr)
		}
		return
	}

	if err := WriteJSON(w, http.StatusCreated, doc); err != nil {
		h.logWriteError(r, err)
	}
}

// GetDocument handles GET /documents/get to fetch a document by id.
func (h Handler) GetDocument(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		if err := WriteError(w, http.StatusMethodNotAllowed, methodNotAllowed); err != nil {
			h.logWriteError(r, err)
		}
		return
	}

	documentID := r.URL.Query().Get("id")
	if documentID == "" {
		if err := WriteError(w, http.StatusBadRequest, "id is required"); err != nil {
			h.logWriteError(r, err)
		}
		return
	}

	ctx := r.Context()
	doc, err := h.getDocument.Execute(ctx, documentID)
	if err != nil {
		if werr := WriteError(w, http.StatusNotFound, "document not found"); werr != nil {
			h.logWriteError(r, werr)
		}
		return
	}

	if err := WriteJSON(w, http.StatusOK, doc); err != nil {
		h.logWriteError(r, err)
	}
}

// DeleteDocument handles DELETE /documents/delete to remove a document by id.
func (h Handler) DeleteDocument(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodDelete {
		if err := WriteError(w, http.StatusMethodNotAllowed, "method not allowed"); err != nil {
			h.logWriteError(r, err)
		}
		return
	}

	documentID := r.URL.Query().Get("id")
	if documentID == "" {
		if err := WriteError(w, http.StatusBadRequest, "id is required"); err != nil {
			h.logWriteError(r, err)
		}
		return
	}

	ctx := r.Context()
	if err := h.deleteDocument.Execute(ctx, usecase.DeleteDocumentInput{
		DocumentID: documentID,
		ActorID:    contextx.Get(ctx, contextx.UserIDKey),
		RequestID:  contextx.Get(ctx, contextx.RequestIDKey),
	}); err != nil {
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
