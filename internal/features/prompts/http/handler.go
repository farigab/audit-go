// Package http handles prompt and chatbot endpoints.
package http

import (
	"encoding/json"
	"errors"
	nethttp "net/http"

	"github.com/rs/zerolog"

	"audit-go/internal/features/access"
	"audit-go/internal/features/prompts/app"
	"audit-go/internal/platform/contextx"
	"audit-go/internal/platform/httpx"
)

type Handler struct {
	log              zerolog.Logger
	createPrompt     app.CreatePromptUseCase
	listPrompts      app.ListPromptsUseCase
	createVersion    app.CreateVersionUseCase
	listVersions     app.ListVersionsUseCase
	approveVersion   app.ApproveVersionUseCase
	deprecateVersion app.DeprecateVersionUseCase
	chat             app.ChatUseCase
	listRuns         app.ListRunsUseCase
}

func NewHandler(
	log zerolog.Logger,
	createPrompt app.CreatePromptUseCase,
	listPrompts app.ListPromptsUseCase,
	createVersion app.CreateVersionUseCase,
	listVersions app.ListVersionsUseCase,
	approveVersion app.ApproveVersionUseCase,
	deprecateVersion app.DeprecateVersionUseCase,
	chat app.ChatUseCase,
	listRuns app.ListRunsUseCase,
) Handler {
	return Handler{
		log:              log,
		createPrompt:     createPrompt,
		listPrompts:      listPrompts,
		createVersion:    createVersion,
		listVersions:     listVersions,
		approveVersion:   approveVersion,
		deprecateVersion: deprecateVersion,
		chat:             chat,
		listRuns:         listRuns,
	}
}

func RegisterRoutes(mux *nethttp.ServeMux, auth func(nethttp.Handler) nethttp.Handler, h Handler) {
	mux.Handle("GET /prompts", auth(nethttp.HandlerFunc(h.ListPrompts)))
	mux.Handle("POST /prompts", auth(nethttp.HandlerFunc(h.CreatePrompt)))
	mux.Handle("GET /prompts/{promptID}/versions", auth(nethttp.HandlerFunc(h.ListVersions)))
	mux.Handle("POST /prompts/{promptID}/versions", auth(nethttp.HandlerFunc(h.CreateVersion)))
	mux.Handle("POST /prompt-versions/{versionID}/approve", auth(nethttp.HandlerFunc(h.ApproveVersion)))
	mux.Handle("POST /prompt-versions/{versionID}/deprecate", auth(nethttp.HandlerFunc(h.DeprecateVersion)))
	mux.Handle("POST /chat", auth(nethttp.HandlerFunc(h.Chat)))
	mux.Handle("GET /prompt-runs", auth(nethttp.HandlerFunc(h.ListRuns)))
}

func (h Handler) CreatePrompt(w nethttp.ResponseWriter, r *nethttp.Request) {
	principal, ok := access.PrincipalFromContext(r.Context())
	if !ok {
		h.writeError(w, r, nethttp.StatusUnauthorized, "unauthenticated")
		return
	}
	var input app.CreatePromptInput
	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		h.writeError(w, r, nethttp.StatusBadRequest, "invalid json body")
		return
	}
	prompt, err := h.createPrompt.Execute(r.Context(), principal, input)
	if err != nil {
		h.writeUseCaseError(w, r, err)
		return
	}
	if err := httpx.WriteJSON(w, nethttp.StatusCreated, prompt); err != nil {
		h.logWriteError(r, err)
	}
}

func (h Handler) ListPrompts(w nethttp.ResponseWriter, r *nethttp.Request) {
	prompts, err := h.listPrompts.Execute(r.Context())
	if err != nil {
		h.writeUseCaseError(w, r, err)
		return
	}
	if err := httpx.WriteJSON(w, nethttp.StatusOK, prompts); err != nil {
		h.logWriteError(r, err)
	}
}

func (h Handler) CreateVersion(w nethttp.ResponseWriter, r *nethttp.Request) {
	principal, ok := access.PrincipalFromContext(r.Context())
	if !ok {
		h.writeError(w, r, nethttp.StatusUnauthorized, "unauthenticated")
		return
	}
	var input app.CreateVersionInput
	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		h.writeError(w, r, nethttp.StatusBadRequest, "invalid json body")
		return
	}
	input.PromptID = r.PathValue("promptID")
	version, err := h.createVersion.Execute(r.Context(), principal, input)
	if err != nil {
		h.writeUseCaseError(w, r, err)
		return
	}
	if err := httpx.WriteJSON(w, nethttp.StatusCreated, version); err != nil {
		h.logWriteError(r, err)
	}
}

func (h Handler) ListVersions(w nethttp.ResponseWriter, r *nethttp.Request) {
	versions, err := h.listVersions.Execute(r.Context(), r.PathValue("promptID"))
	if err != nil {
		h.writeUseCaseError(w, r, err)
		return
	}
	if err := httpx.WriteJSON(w, nethttp.StatusOK, versions); err != nil {
		h.logWriteError(r, err)
	}
}

func (h Handler) ApproveVersion(w nethttp.ResponseWriter, r *nethttp.Request) {
	principal, ok := access.PrincipalFromContext(r.Context())
	if !ok {
		h.writeError(w, r, nethttp.StatusUnauthorized, "unauthenticated")
		return
	}
	if err := h.approveVersion.Execute(r.Context(), principal, r.PathValue("versionID")); err != nil {
		h.writeUseCaseError(w, r, err)
		return
	}
	if err := httpx.WriteJSON(w, nethttp.StatusOK, map[string]string{"status": "approved"}); err != nil {
		h.logWriteError(r, err)
	}
}

func (h Handler) DeprecateVersion(w nethttp.ResponseWriter, r *nethttp.Request) {
	principal, ok := access.PrincipalFromContext(r.Context())
	if !ok {
		h.writeError(w, r, nethttp.StatusUnauthorized, "unauthenticated")
		return
	}
	if err := h.deprecateVersion.Execute(r.Context(), principal, r.PathValue("versionID")); err != nil {
		h.writeUseCaseError(w, r, err)
		return
	}
	if err := httpx.WriteJSON(w, nethttp.StatusOK, map[string]string{"status": "deprecated"}); err != nil {
		h.logWriteError(r, err)
	}
}

func (h Handler) Chat(w nethttp.ResponseWriter, r *nethttp.Request) {
	principal, ok := access.PrincipalFromContext(r.Context())
	if !ok {
		h.writeError(w, r, nethttp.StatusUnauthorized, "unauthenticated")
		return
	}
	var input app.ChatInput
	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		h.writeError(w, r, nethttp.StatusBadRequest, "invalid json body")
		return
	}
	result, err := h.chat.Execute(r.Context(), principal, input)
	if err != nil {
		h.writeUseCaseError(w, r, err)
		return
	}
	if err := httpx.WriteJSON(w, nethttp.StatusOK, result); err != nil {
		h.logWriteError(r, err)
	}
}

func (h Handler) ListRuns(w nethttp.ResponseWriter, r *nethttp.Request) {
	principal, ok := access.PrincipalFromContext(r.Context())
	if !ok {
		h.writeError(w, r, nethttp.StatusUnauthorized, "unauthenticated")
		return
	}
	runs, err := h.listRuns.Execute(r.Context(), principal, r.URL.Query().Get("jv_id"))
	if err != nil {
		h.writeUseCaseError(w, r, err)
		return
	}
	if err := httpx.WriteJSON(w, nethttp.StatusOK, runs); err != nil {
		h.logWriteError(r, err)
	}
}

func (h Handler) writeUseCaseError(w nethttp.ResponseWriter, r *nethttp.Request, err error) {
	switch {
	case errors.Is(err, app.ErrInvalidInput):
		h.writeError(w, r, nethttp.StatusBadRequest, "invalid input")
	case errors.Is(err, app.ErrNotFound):
		h.writeError(w, r, nethttp.StatusNotFound, "not found")
	case errors.Is(err, app.ErrForbidden), errors.Is(err, access.ErrForbidden):
		h.writeError(w, r, nethttp.StatusForbidden, "forbidden")
	case errors.Is(err, access.ErrUnauthenticated):
		h.writeError(w, r, nethttp.StatusUnauthorized, "unauthenticated")
	default:
		requestID := contextx.Get(r.Context(), contextx.RequestIDKey)
		h.log.Error().Err(err).Str("path", r.URL.Path).Str("request_id", requestID).Msg("prompt handler error")
		h.writeError(w, r, nethttp.StatusInternalServerError, "internal error")
	}
}

func (h Handler) writeError(w nethttp.ResponseWriter, r *nethttp.Request, status int, message string) {
	if err := httpx.WriteError(w, status, message); err != nil {
		h.logWriteError(r, err)
	}
}

func (h Handler) logWriteError(r *nethttp.Request, err error) {
	h.log.Error().Err(err).Str("path", r.URL.Path).Msg("failed to write response")
}
