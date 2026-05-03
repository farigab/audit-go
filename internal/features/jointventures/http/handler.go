// Package http exposes joint venture HTTP handlers.
package http

import (
	"encoding/json"
	"errors"
	nethttp "net/http"

	"github.com/rs/zerolog"

	"audit-go/internal/features/access"
	"audit-go/internal/features/jointventures"
	"audit-go/internal/features/jointventures/app"
	"audit-go/internal/platform/httpx"
	"audit-go/internal/platform/httpx/middleware"
)

// Handler handles joint venture endpoints.
type Handler struct {
	log             zerolog.Logger
	createJV        app.CreateJointVentureUseCase
	listJVsByRegion app.ListJointVenturesByRegionUseCase
	getJV           app.GetJointVentureUseCase
	updateJV        app.UpdateJointVentureUseCase
	deleteJV        app.DeleteJointVentureUseCase
}

// NewHandler creates a joint venture handler.
func NewHandler(
	log zerolog.Logger,
	create app.CreateJointVentureUseCase,
	listByRegion app.ListJointVenturesByRegionUseCase,
	get app.GetJointVentureUseCase,
	update app.UpdateJointVentureUseCase,
	del app.DeleteJointVentureUseCase,
) Handler {
	return Handler{
		log:             log,
		createJV:        create,
		listJVsByRegion: listByRegion,
		getJV:           get,
		updateJV:        update,
		deleteJV:        del,
	}
}

// RegisterRoutes wires authenticated joint venture routes.
func RegisterRoutes(
	mux *nethttp.ServeMux,
	auth func(nethttp.Handler) nethttp.Handler,
	authorizer middleware.PermissionAuthorizer,
	h Handler,
) {
	require := func(
		permission access.Permission,
		scope middleware.ScopeResolver,
		handler nethttp.HandlerFunc,
	) nethttp.Handler {
		return auth(middleware.RequirePermission(authorizer, permission, scope)(nethttp.HandlerFunc(handler)))
	}

	mux.Handle("POST /regions/{regionID}/joint-ventures", require(access.PermissionJVCreate, middleware.RegionScopeFromPath("regionID"), h.CreateJointVenture))
	mux.Handle("GET /regions/{regionID}/joint-ventures", auth(nethttp.HandlerFunc(h.ListJointVenturesByRegion)))
	mux.Handle("GET /joint-ventures/{jvID}", require(access.PermissionJVRead, middleware.JVScopeFromPath("jvID"), h.GetJointVenture))
	mux.Handle("PATCH /joint-ventures/{jvID}", require(access.PermissionJVUpdate, middleware.JVScopeFromPath("jvID"), h.UpdateJointVenture))
	mux.Handle("DELETE /joint-ventures/{jvID}", require(access.PermissionJVDelete, middleware.JVScopeFromPath("jvID"), h.DeleteJointVenture))
}

// CreateJointVenture handles POST /regions/{regionID}/joint-ventures.
func (h Handler) CreateJointVenture(w nethttp.ResponseWriter, r *nethttp.Request) {
	regionID := r.PathValue("regionID")
	if regionID == "" {
		h.writeError(w, r, nethttp.StatusBadRequest, "regionID is required")
		return
	}

	var body struct {
		Name     string            `json:"name"`
		Parties  []string          `json:"parties"`
		Metadata map[string]string `json:"metadata"`
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

	jv, err := h.createJV.Execute(r.Context(), principal, app.CreateJointVentureInput{
		RegionID: regionID,
		Name:     body.Name,
		Parties:  body.Parties,
		Metadata: body.Metadata,
	})
	if err != nil {
		h.writeUseCaseError(w, r, err)
		return
	}

	if err := httpx.WriteJSON(w, nethttp.StatusCreated, jv); err != nil {
		h.logWriteError(r, err)
	}
}

// ListJointVenturesByRegion handles GET /regions/{regionID}/joint-ventures.
func (h Handler) ListJointVenturesByRegion(w nethttp.ResponseWriter, r *nethttp.Request) {
	regionID := r.PathValue("regionID")
	if regionID == "" {
		h.writeError(w, r, nethttp.StatusBadRequest, "regionID is required")
		return
	}

	principal, ok := access.PrincipalFromContext(r.Context())
	if !ok {
		h.writeError(w, r, nethttp.StatusUnauthorized, "unauthorized")
		return
	}

	jvs, err := h.listJVsByRegion.Execute(r.Context(), principal, regionID)
	if err != nil {
		h.writeUseCaseError(w, r, err)
		return
	}

	if err := httpx.WriteJSON(w, nethttp.StatusOK, jvs); err != nil {
		h.logWriteError(r, err)
	}
}

// GetJointVenture handles GET /joint-ventures/{jvID}.
func (h Handler) GetJointVenture(w nethttp.ResponseWriter, r *nethttp.Request) {
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

	jv, err := h.getJV.Execute(r.Context(), principal, jvID)
	if err != nil {
		h.writeUseCaseError(w, r, err)
		return
	}

	if err := httpx.WriteJSON(w, nethttp.StatusOK, jv); err != nil {
		h.logWriteError(r, err)
	}
}

// UpdateJointVenture handles PATCH /joint-ventures/{jvID}.
func (h Handler) UpdateJointVenture(w nethttp.ResponseWriter, r *nethttp.Request) {
	jvID := r.PathValue("jvID")
	if jvID == "" {
		h.writeError(w, r, nethttp.StatusBadRequest, "jvID is required")
		return
	}

	var body struct {
		Name     *string               `json:"name"`
		Parties  *[]string             `json:"parties"`
		Status   *jointventures.Status `json:"status"`
		Metadata map[string]string     `json:"metadata"`
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

	var parties []string
	if body.Parties != nil {
		parties = *body.Parties
	}

	jv, err := h.updateJV.Execute(r.Context(), principal, app.UpdateJointVentureInput{
		ID:       jvID,
		Name:     body.Name,
		Parties:  parties,
		Status:   body.Status,
		Metadata: body.Metadata,
	})
	if err != nil {
		h.writeUseCaseError(w, r, err)
		return
	}

	if err := httpx.WriteJSON(w, nethttp.StatusOK, jv); err != nil {
		h.logWriteError(r, err)
	}
}

// DeleteJointVenture handles DELETE /joint-ventures/{jvID}.
func (h Handler) DeleteJointVenture(w nethttp.ResponseWriter, r *nethttp.Request) {
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

	if err := h.deleteJV.Execute(r.Context(), principal, jvID); err != nil {
		h.writeUseCaseError(w, r, err)
		return
	}

	if err := httpx.WriteJSON(w, nethttp.StatusOK, map[string]string{
		"status": "deleted",
		"id":     jvID,
	}); err != nil {
		h.logWriteError(r, err)
	}
}

func (h Handler) writeUseCaseError(w nethttp.ResponseWriter, r *nethttp.Request, err error) {
	switch {
	case errors.Is(err, app.ErrInvalidInput):
		h.writeError(w, r, nethttp.StatusBadRequest, "invalid joint venture request")
	case errors.Is(err, app.ErrNotFound):
		h.writeError(w, r, nethttp.StatusNotFound, "joint venture not found")
	case errors.Is(err, access.ErrUnauthenticated):
		h.writeError(w, r, nethttp.StatusUnauthorized, "unauthorized")
	case errors.Is(err, access.ErrForbidden):
		h.writeError(w, r, nethttp.StatusForbidden, "forbidden")
	default:
		h.log.Error().Err(err).Str("path", r.URL.Path).Msg("joint venture handler error")
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
