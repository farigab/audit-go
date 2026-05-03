// Package http exposes document HTTP handlers.
package http

import (
	"encoding/json"
	"errors"
	"io"
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
	log                    zerolog.Logger
	createDocument         app.CreateDocumentUseCase
	requestDocumentUpload  app.RequestDocumentUploadUseCase
	completeDocumentUpload app.CompleteDocumentUploadUseCase
	deleteDocument         app.DeleteDocumentUseCase
	getDocument            app.GetDocumentUseCase
	getProcessingStatus    app.GetDocumentProcessingStatusUseCase
	listDocuments          app.ListDocumentsByJVUseCase
}

// NewHandler creates a document handler wired with the provided use cases.
func NewHandler(
	log zerolog.Logger,
	create app.CreateDocumentUseCase,
	requestUpload app.RequestDocumentUploadUseCase,
	completeUpload app.CompleteDocumentUploadUseCase,
	del app.DeleteDocumentUseCase,
	get app.GetDocumentUseCase,
	getProcessingStatus app.GetDocumentProcessingStatusUseCase,
	list app.ListDocumentsByJVUseCase,
) Handler {
	return Handler{
		log:                    log,
		createDocument:         create,
		requestDocumentUpload:  requestUpload,
		completeDocumentUpload: completeUpload,
		deleteDocument:         del,
		getDocument:            get,
		getProcessingStatus:    getProcessingStatus,
		listDocuments:          list,
	}
}

// RegisterRoutes wires authenticated document routes.
func RegisterRoutes(mux *nethttp.ServeMux, auth func(nethttp.Handler) nethttp.Handler, h Handler) {
	mux.Handle("POST /documents", auth(nethttp.HandlerFunc(h.CreateDocument)))
	mux.Handle("POST /joint-ventures/{jvID}/documents/upload-url", auth(nethttp.HandlerFunc(h.RequestDocumentUpload)))
	mux.Handle("POST /documents/{documentID}/upload-complete", auth(nethttp.HandlerFunc(h.CompleteDocumentUpload)))
	mux.Handle("GET /documents/{documentID}/processing-status", auth(nethttp.HandlerFunc(h.GetDocumentProcessingStatus)))
	mux.Handle("GET /documents/get", auth(nethttp.HandlerFunc(h.GetDocument)))
	mux.Handle("DELETE /documents/delete", auth(nethttp.HandlerFunc(h.DeleteDocument)))
	mux.Handle("GET /joint-ventures/{jvID}/documents", auth(nethttp.HandlerFunc(h.ListDocumentsByJV)))
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

// RequestDocumentUpload handles POST /joint-ventures/{jvID}/documents/upload-url.
func (h Handler) RequestDocumentUpload(w nethttp.ResponseWriter, r *nethttp.Request) {
	jvID := r.PathValue("jvID")
	if jvID == "" {
		h.writeError(w, r, nethttp.StatusBadRequest, "jvID is required")
		return
	}

	var body struct {
		Filename    string         `json:"filename"`
		Name        string         `json:"name"`
		Type        documents.Type `json:"type"`
		ContentType string         `json:"content_type"`
		SizeBytes   *int64         `json:"size_bytes"`
	}

	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		h.writeError(w, r, nethttp.StatusBadRequest, "invalid request body")
		return
	}

	filename := body.Filename
	if filename == "" {
		filename = body.Name
	}

	principal, ok := access.PrincipalFromContext(r.Context())
	if !ok {
		h.writeError(w, r, nethttp.StatusUnauthorized, "unauthorized")
		return
	}

	output, err := h.requestDocumentUpload.Execute(r.Context(), principal, app.RequestDocumentUploadInput{
		JVID:        jvID,
		RequestID:   contextx.Get(r.Context(), contextx.RequestIDKey),
		Filename:    filename,
		Type:        body.Type,
		ContentType: body.ContentType,
		SizeBytes:   body.SizeBytes,
	})
	if err != nil {
		h.writeUseCaseError(w, r, err)
		return
	}

	if err = httpx.WriteJSON(w, nethttp.StatusCreated, output); err != nil {
		h.logWriteError(r, err)
	}
}

// CompleteDocumentUpload handles POST /documents/{documentID}/upload-complete.
func (h Handler) CompleteDocumentUpload(w nethttp.ResponseWriter, r *nethttp.Request) {
	documentID := r.PathValue("documentID")
	if documentID == "" {
		h.writeError(w, r, nethttp.StatusBadRequest, "documentID is required")
		return
	}

	var body struct {
		SizeBytes *int64 `json:"size_bytes"`
	}

	if r.Body != nil {
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			if !errors.Is(err, io.EOF) {
				h.writeError(w, r, nethttp.StatusBadRequest, "invalid request body")
				return
			}
		}
	}

	principal, ok := access.PrincipalFromContext(r.Context())
	if !ok {
		h.writeError(w, r, nethttp.StatusUnauthorized, "unauthorized")
		return
	}

	doc, err := h.completeDocumentUpload.Execute(r.Context(), principal, app.CompleteDocumentUploadInput{
		DocumentID: documentID,
		RequestID:  contextx.Get(r.Context(), contextx.RequestIDKey),
		SizeBytes:  body.SizeBytes,
	})
	if err != nil {
		h.writeUseCaseError(w, r, err)
		return
	}

	if err = httpx.WriteJSON(w, nethttp.StatusOK, doc); err != nil {
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

// GetDocumentProcessingStatus handles GET /documents/{documentID}/processing-status.
func (h Handler) GetDocumentProcessingStatus(w nethttp.ResponseWriter, r *nethttp.Request) {
	documentID := r.PathValue("documentID")
	if documentID == "" {
		h.writeError(w, r, nethttp.StatusBadRequest, "documentID is required")
		return
	}

	principal, ok := access.PrincipalFromContext(r.Context())
	if !ok {
		h.writeError(w, r, nethttp.StatusUnauthorized, "unauthorized")
		return
	}

	status, err := h.getProcessingStatus.Execute(r.Context(), principal, documentID)
	if err != nil {
		h.writeUseCaseError(w, r, err)
		return
	}

	if err = httpx.WriteJSON(w, nethttp.StatusOK, status); err != nil {
		h.logWriteError(r, err)
	}
}

// ListDocumentsByJV handles GET /joint-ventures/{jvID}/documents.
func (h Handler) ListDocumentsByJV(w nethttp.ResponseWriter, r *nethttp.Request) {
	jvID := r.PathValue("jvID")
	if jvID == "" {
		h.writeError(w, r, nethttp.StatusBadRequest, "jvID is required")
		return
	}

	principal, ok := access.PrincipalFromContext(r.Context())
	if !ok {
		h.writeError(w, r, nethttp.StatusUnauthorized, "unauthorized")
		return
	}

	docs, err := h.listDocuments.Execute(r.Context(), principal, jvID)
	if err != nil {
		h.writeUseCaseError(w, r, err)
		return
	}

	if err = httpx.WriteJSON(w, nethttp.StatusOK, docs); err != nil {
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
	case errors.Is(err, app.ErrBlobNotFound):
		h.writeError(w, r, nethttp.StatusConflict, "uploaded file was not found")
	case errors.Is(err, app.ErrStorageNotConfigured):
		h.writeError(w, r, nethttp.StatusServiceUnavailable, "document storage is not configured")
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
