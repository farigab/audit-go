package http

import (
	"encoding/json"
	"errors"
	nethttp "net/http"

	"github.com/rs/zerolog"

	"audit-go/internal/features/access"
	accessapp "audit-go/internal/features/access/app"
	"audit-go/internal/platform/httpx"
)

// MembershipHandler handles access membership endpoints.
type MembershipHandler struct {
	log              zerolog.Logger
	createMembership accessapp.CreateMembershipUseCase
	listMemberships  accessapp.ListMembershipsUseCase
	deleteMembership accessapp.DeleteMembershipUseCase
}

// NewMembershipHandler creates a membership handler.
func NewMembershipHandler(
	log zerolog.Logger,
	create accessapp.CreateMembershipUseCase,
	list accessapp.ListMembershipsUseCase,
	del accessapp.DeleteMembershipUseCase,
) MembershipHandler {
	return MembershipHandler{
		log:              log,
		createMembership: create,
		listMemberships:  list,
		deleteMembership: del,
	}
}

// RegisterMembershipRoutes wires authenticated membership routes.
func RegisterMembershipRoutes(mux *nethttp.ServeMux, auth func(nethttp.Handler) nethttp.Handler, h MembershipHandler) {
	mux.Handle("POST /access/memberships", auth(nethttp.HandlerFunc(h.CreateMembership)))
	mux.Handle("GET /access/memberships", auth(nethttp.HandlerFunc(h.ListMemberships)))
	mux.Handle("DELETE /access/memberships/{membershipID}", auth(nethttp.HandlerFunc(h.DeleteMembership)))
}

// CreateMembership handles POST /access/memberships.
func (h MembershipHandler) CreateMembership(w nethttp.ResponseWriter, r *nethttp.Request) {
	var body struct {
		UserLogin string           `json:"user_login"`
		Role      access.Role      `json:"role"`
		ScopeType access.ScopeType `json:"scope_type"`
		ScopeID   string           `json:"scope_id"`
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

	membership, err := h.createMembership.Execute(r.Context(), principal, accessapp.CreateMembershipInput{
		UserLogin: body.UserLogin,
		Role:      body.Role,
		ScopeType: body.ScopeType,
		ScopeID:   body.ScopeID,
	})
	if err != nil {
		h.writeUseCaseError(w, r, err)
		return
	}

	if err := httpx.WriteJSON(w, nethttp.StatusCreated, membership); err != nil {
		h.logWriteError(r, err)
	}
}

// ListMemberships handles GET /access/memberships.
func (h MembershipHandler) ListMemberships(w nethttp.ResponseWriter, r *nethttp.Request) {
	principal, ok := access.PrincipalFromContext(r.Context())
	if !ok {
		h.writeError(w, r, nethttp.StatusUnauthorized, "unauthorized")
		return
	}

	memberships, err := h.listMemberships.Execute(r.Context(), principal, accessapp.MembershipFilter{
		UserLogin: r.URL.Query().Get("user_login"),
		ScopeType: access.ScopeType(r.URL.Query().Get("scope_type")),
		ScopeID:   r.URL.Query().Get("scope_id"),
	})
	if err != nil {
		h.writeUseCaseError(w, r, err)
		return
	}

	if err := httpx.WriteJSON(w, nethttp.StatusOK, memberships); err != nil {
		h.logWriteError(r, err)
	}
}

// DeleteMembership handles DELETE /access/memberships/{membershipID}.
func (h MembershipHandler) DeleteMembership(w nethttp.ResponseWriter, r *nethttp.Request) {
	membershipID := r.PathValue("membershipID")
	if membershipID == "" {
		h.writeError(w, r, nethttp.StatusBadRequest, "membershipID is required")
		return
	}

	principal, ok := access.PrincipalFromContext(r.Context())
	if !ok {
		h.writeError(w, r, nethttp.StatusUnauthorized, "unauthorized")
		return
	}

	if err := h.deleteMembership.Execute(r.Context(), principal, membershipID); err != nil {
		h.writeUseCaseError(w, r, err)
		return
	}

	if err := httpx.WriteJSON(w, nethttp.StatusOK, map[string]string{
		"status": "deleted",
		"id":     membershipID,
	}); err != nil {
		h.logWriteError(r, err)
	}
}

func (h MembershipHandler) writeUseCaseError(w nethttp.ResponseWriter, r *nethttp.Request, err error) {
	switch {
	case errors.Is(err, accessapp.ErrInvalidMembership):
		h.writeError(w, r, nethttp.StatusBadRequest, "invalid membership request")
	case errors.Is(err, accessapp.ErrMembershipNotFound):
		h.writeError(w, r, nethttp.StatusNotFound, "membership not found")
	case errors.Is(err, access.ErrUnauthenticated):
		h.writeError(w, r, nethttp.StatusUnauthorized, "unauthorized")
	case errors.Is(err, access.ErrForbidden):
		h.writeError(w, r, nethttp.StatusForbidden, "forbidden")
	default:
		h.log.Error().Err(err).Str("path", r.URL.Path).Msg("membership handler error")
		h.writeError(w, r, nethttp.StatusInternalServerError, "internal server error")
	}
}

func (h MembershipHandler) writeError(w nethttp.ResponseWriter, r *nethttp.Request, status int, message string) {
	if err := httpx.WriteError(w, status, message); err != nil {
		h.logWriteError(r, err)
	}
}

func (h MembershipHandler) logWriteError(r *nethttp.Request, err error) {
	h.log.Error().
		Err(err).
		Str("path", r.URL.Path).
		Msg("failed to write response")
}
