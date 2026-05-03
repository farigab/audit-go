// Package http exposes region HTTP handlers.
package http

import (
	"encoding/json"
	"errors"
	nethttp "net/http"

	"github.com/rs/zerolog"

	"audit-go/internal/features/access"
	"audit-go/internal/features/regions/app"
	"audit-go/internal/platform/httpx"
	"audit-go/internal/platform/httpx/middleware"
)

// Handler handles region endpoints.
type Handler struct {
	log          zerolog.Logger
	createRegion app.CreateRegionUseCase
	listRegions  app.ListRegionsUseCase
	getRegion    app.GetRegionUseCase
	updateRegion app.UpdateRegionUseCase
	deleteRegion app.DeleteRegionUseCase
}

// NewHandler creates a region handler.
func NewHandler(
	log zerolog.Logger,
	create app.CreateRegionUseCase,
	list app.ListRegionsUseCase,
	get app.GetRegionUseCase,
	update app.UpdateRegionUseCase,
	del app.DeleteRegionUseCase,
) Handler {
	return Handler{
		log:          log,
		createRegion: create,
		listRegions:  list,
		getRegion:    get,
		updateRegion: update,
		deleteRegion: del,
	}
}

// RegisterRoutes wires authenticated region routes.
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

	mux.Handle("POST /regions", require(access.PermissionRegionCreate, middleware.SystemScope(), h.CreateRegion))
	mux.Handle("GET /regions", auth(nethttp.HandlerFunc(h.ListRegions)))
	mux.Handle("GET /regions/{regionID}", require(access.PermissionRegionRead, middleware.RegionScopeFromPath("regionID"), h.GetRegion))
	mux.Handle("PATCH /regions/{regionID}", require(access.PermissionRegionUpdate, middleware.RegionScopeFromPath("regionID"), h.UpdateRegion))
	mux.Handle("DELETE /regions/{regionID}", require(access.PermissionRegionDelete, middleware.SystemScope(), h.DeleteRegion))
}

// CreateRegion handles POST /regions.
func (h Handler) CreateRegion(w nethttp.ResponseWriter, r *nethttp.Request) {
	var body struct {
		Name string `json:"name"`
		Code string `json:"code"`
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

	region, err := h.createRegion.Execute(r.Context(), principal, app.CreateRegionInput{
		Name: body.Name,
		Code: body.Code,
	})
	if err != nil {
		h.writeUseCaseError(w, r, err)
		return
	}

	if err := httpx.WriteJSON(w, nethttp.StatusCreated, region); err != nil {
		h.logWriteError(r, err)
	}
}

// ListRegions handles GET /regions.
func (h Handler) ListRegions(w nethttp.ResponseWriter, r *nethttp.Request) {
	principal, ok := access.PrincipalFromContext(r.Context())
	if !ok {
		h.writeError(w, r, nethttp.StatusUnauthorized, "unauthorized")
		return
	}

	regions, err := h.listRegions.Execute(r.Context(), principal)
	if err != nil {
		h.writeUseCaseError(w, r, err)
		return
	}

	if err := httpx.WriteJSON(w, nethttp.StatusOK, regions); err != nil {
		h.logWriteError(r, err)
	}
}

// GetRegion handles GET /regions/{regionID}.
func (h Handler) GetRegion(w nethttp.ResponseWriter, r *nethttp.Request) {
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

	region, err := h.getRegion.Execute(r.Context(), principal, regionID)
	if err != nil {
		h.writeUseCaseError(w, r, err)
		return
	}

	if err := httpx.WriteJSON(w, nethttp.StatusOK, region); err != nil {
		h.logWriteError(r, err)
	}
}

// UpdateRegion handles PATCH /regions/{regionID}.
func (h Handler) UpdateRegion(w nethttp.ResponseWriter, r *nethttp.Request) {
	regionID := r.PathValue("regionID")
	if regionID == "" {
		h.writeError(w, r, nethttp.StatusBadRequest, "regionID is required")
		return
	}

	var body struct {
		Name string `json:"name"`
		Code string `json:"code"`
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

	region, err := h.updateRegion.Execute(r.Context(), principal, app.UpdateRegionInput{
		ID:   regionID,
		Name: body.Name,
		Code: body.Code,
	})
	if err != nil {
		h.writeUseCaseError(w, r, err)
		return
	}

	if err := httpx.WriteJSON(w, nethttp.StatusOK, region); err != nil {
		h.logWriteError(r, err)
	}
}

// DeleteRegion handles DELETE /regions/{regionID}.
func (h Handler) DeleteRegion(w nethttp.ResponseWriter, r *nethttp.Request) {
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

	if err := h.deleteRegion.Execute(r.Context(), principal, regionID); err != nil {
		h.writeUseCaseError(w, r, err)
		return
	}

	if err := httpx.WriteJSON(w, nethttp.StatusOK, map[string]string{
		"status": "deleted",
		"id":     regionID,
	}); err != nil {
		h.logWriteError(r, err)
	}
}

func (h Handler) writeUseCaseError(w nethttp.ResponseWriter, r *nethttp.Request, err error) {
	switch {
	case errors.Is(err, app.ErrInvalidInput):
		h.writeError(w, r, nethttp.StatusBadRequest, "invalid region request")
	case errors.Is(err, app.ErrNotFound):
		h.writeError(w, r, nethttp.StatusNotFound, "region not found")
	case errors.Is(err, access.ErrUnauthenticated):
		h.writeError(w, r, nethttp.StatusUnauthorized, "unauthorized")
	case errors.Is(err, access.ErrForbidden):
		h.writeError(w, r, nethttp.StatusForbidden, "forbidden")
	default:
		h.log.Error().Err(err).Str("path", r.URL.Path).Msg("region handler error")
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
