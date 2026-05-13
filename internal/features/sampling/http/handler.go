// Package http exposes sampling HTTP handlers.
package http

import (
	"encoding/json"
	"errors"
	nethttp "net/http"
	"strconv"

	"github.com/rs/zerolog"

	"audit-go/internal/features/access"
	"audit-go/internal/features/sampling"
	"audit-go/internal/features/sampling/app"
	"audit-go/internal/platform/contextx"
	"audit-go/internal/platform/httpx"
)

// Handler handles sampling endpoints.
type Handler struct {
	log           zerolog.Logger
	createRuleSet app.CreateRuleSetUseCase
	listRuleSets  app.ListRuleSetsUseCase
	preview       app.PreviewUseCase
	createRun     app.CreateRunUseCase
	listRuns      app.ListRunsUseCase
}

// NewHandler creates a sampling handler.
func NewHandler(
	log zerolog.Logger,
	createRuleSet app.CreateRuleSetUseCase,
	listRuleSets app.ListRuleSetsUseCase,
	preview app.PreviewUseCase,
	createRun app.CreateRunUseCase,
	listRuns app.ListRunsUseCase,
) Handler {
	return Handler{
		log:           log,
		createRuleSet: createRuleSet,
		listRuleSets:  listRuleSets,
		preview:       preview,
		createRun:     createRun,
		listRuns:      listRuns,
	}
}

// RegisterRoutes wires authenticated sampling routes.
func RegisterRoutes(mux *nethttp.ServeMux, auth func(nethttp.Handler) nethttp.Handler, h Handler) {
	mux.Handle("GET /joint-ventures/{jvID}/sampling/rule-sets", auth(nethttp.HandlerFunc(h.ListRuleSets)))
	mux.Handle("POST /joint-ventures/{jvID}/sampling/rule-sets", auth(nethttp.HandlerFunc(h.CreateRuleSet)))
	mux.Handle("POST /joint-ventures/{jvID}/sampling/preview", auth(nethttp.HandlerFunc(h.Preview)))
	mux.Handle("GET /joint-ventures/{jvID}/sampling/runs", auth(nethttp.HandlerFunc(h.ListRuns)))
	mux.Handle("POST /joint-ventures/{jvID}/sampling/runs", auth(nethttp.HandlerFunc(h.CreateRun)))
}

// CreateRuleSet handles POST /joint-ventures/{jvID}/sampling/rule-sets.
func (h Handler) CreateRuleSet(w nethttp.ResponseWriter, r *nethttp.Request) {
	jvID := r.PathValue("jvID")
	var body struct {
		Name             string              `json:"name"`
		Description      string              `json:"description"`
		Parameters       sampling.Parameters `json:"parameters"`
		QualitativeRules []string            `json:"qualitative_rules"`
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
	ruleSet, err := h.createRuleSet.Execute(r.Context(), principal, app.CreateRuleSetInput{
		JVID:             jvID,
		RequestID:        contextx.Get(r.Context(), contextx.RequestIDKey),
		Name:             body.Name,
		Description:      body.Description,
		Parameters:       body.Parameters,
		QualitativeRules: body.QualitativeRules,
	})
	if err != nil {
		h.writeUseCaseError(w, r, err)
		return
	}
	if err := httpx.WriteJSON(w, nethttp.StatusCreated, ruleSet); err != nil {
		h.logWriteError(r, err)
	}
}

// ListRuleSets handles GET /joint-ventures/{jvID}/sampling/rule-sets.
func (h Handler) ListRuleSets(w nethttp.ResponseWriter, r *nethttp.Request) {
	principal, ok := access.PrincipalFromContext(r.Context())
	if !ok {
		h.writeError(w, r, nethttp.StatusUnauthorized, "unauthorized")
		return
	}
	items, err := h.listRuleSets.Execute(r.Context(), principal, r.PathValue("jvID"))
	if err != nil {
		h.writeUseCaseError(w, r, err)
		return
	}
	if err := httpx.WriteJSON(w, nethttp.StatusOK, items); err != nil {
		h.logWriteError(r, err)
	}
}

// Preview handles POST /joint-ventures/{jvID}/sampling/preview.
func (h Handler) Preview(w nethttp.ResponseWriter, r *nethttp.Request) {
	var body struct {
		RuleSetID        string              `json:"rule_set_id"`
		Parameters       sampling.Parameters `json:"parameters"`
		QualitativeRules []string            `json:"qualitative_rules"`
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
	preview, err := h.preview.Execute(r.Context(), principal, app.PreviewInput{
		JVID:             r.PathValue("jvID"),
		RuleSetID:        body.RuleSetID,
		Parameters:       body.Parameters,
		QualitativeRules: body.QualitativeRules,
	})
	if err != nil {
		h.writeUseCaseError(w, r, err)
		return
	}
	if err := httpx.WriteJSON(w, nethttp.StatusOK, preview); err != nil {
		h.logWriteError(r, err)
	}
}

// CreateRun handles POST /joint-ventures/{jvID}/sampling/runs.
func (h Handler) CreateRun(w nethttp.ResponseWriter, r *nethttp.Request) {
	var body struct {
		RuleSetID string `json:"rule_set_id"`
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
	run, err := h.createRun.Execute(r.Context(), principal, app.CreateRunInput{
		JVID:      r.PathValue("jvID"),
		RuleSetID: body.RuleSetID,
		RequestID: contextx.Get(r.Context(), contextx.RequestIDKey),
	})
	if err != nil {
		h.writeUseCaseError(w, r, err)
		return
	}
	if err := httpx.WriteJSON(w, nethttp.StatusCreated, run); err != nil {
		h.logWriteError(r, err)
	}
}

// ListRuns handles GET /joint-ventures/{jvID}/sampling/runs.
func (h Handler) ListRuns(w nethttp.ResponseWriter, r *nethttp.Request) {
	principal, ok := access.PrincipalFromContext(r.Context())
	if !ok {
		h.writeError(w, r, nethttp.StatusUnauthorized, "unauthorized")
		return
	}
	limit, err := strconv.Atoi(r.URL.Query().Get("limit"))
	if err != nil {
		limit = 20
	}
	runs, err := h.listRuns.Execute(r.Context(), principal, r.PathValue("jvID"), limit)
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
		h.writeError(w, r, nethttp.StatusBadRequest, "invalid sampling request")
	case errors.Is(err, app.ErrNotFound):
		h.writeError(w, r, nethttp.StatusNotFound, "sampling resource not found")
	case errors.Is(err, access.ErrUnauthenticated):
		h.writeError(w, r, nethttp.StatusUnauthorized, "unauthorized")
	case errors.Is(err, access.ErrForbidden):
		h.writeError(w, r, nethttp.StatusForbidden, "forbidden")
	default:
		h.log.Error().Err(err).Str("path", r.URL.Path).Msg("sampling handler error")
		h.writeError(w, r, nethttp.StatusInternalServerError, "internal server error")
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
