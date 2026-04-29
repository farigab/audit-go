package http

import (
	nethttp "net/http"

	"audit-go/internal/delivery/http/middleware"
)

// RegisterRoutes wires all HTTP routes and returns the configured mux.
func RegisterRoutes(dep Dependencies) *nethttp.ServeMux {
	mux := nethttp.NewServeMux()

	h := NewHandler(dep.Log, dep.CreateDocument, dep.DeleteDocument, dep.GetDocument)
	authH := NewAuthHandler(dep.Log, dep.Config, dep.Login, dep.Logout, dep.Refresh)
	auth := middleware.Auth(dep.JWT)

	// ── Public ────────────────────────────────────────────────────────────────

	mux.HandleFunc("GET /health", h.Health)
	mux.HandleFunc("POST /auth/login", authH.Login)
	mux.HandleFunc("POST /auth/refresh", authH.Refresh)
	mux.HandleFunc("POST /auth/logout", authH.Logout)

	// ── Authenticated ─────────────────────────────────────────────────────────

	mux.Handle("POST /documents", auth(nethttp.HandlerFunc(h.CreateDocument)))
	mux.Handle("GET /documents/get", auth(nethttp.HandlerFunc(h.GetDocument)))
	mux.Handle("DELETE /documents/delete", auth(nethttp.HandlerFunc(h.DeleteDocument)))

	return mux
}
