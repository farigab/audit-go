// Package http wires HTTP handlers, middleware and dependencies.
package http

import (
	nethttp "net/http"

	"audit-go/internal/delivery/http/middleware"
	"audit-go/internal/platform/config"
	"audit-go/internal/platform/security"
	"audit-go/internal/repository"
)

// RegisterAuthRoutes registers authenticated routes.
func RegisterAuthRoutes(
	mux *nethttp.ServeMux,
	h Handler,
	cfg *config.CookieConfig,
	jwtSvc security.TokenService,
	userRepo repository.UserRepository,
	refreshRepo repository.RefreshTokenRepository,
) {
	auth := middleware.AuthWithRefresh(
		cfg,
		jwtSvc,
		userRepo,
		refreshRepo,
	)

	mux.Handle(
		"GET /documents/get",
		auth(nethttp.HandlerFunc(h.GetDocument)),
	)

	mux.Handle(
		"DELETE /documents/delete",
		auth(nethttp.HandlerFunc(h.DeleteDocument)),
	)

	mux.Handle(
		"POST /documents",
		auth(nethttp.HandlerFunc(h.CreateDocument)),
	)
}
