package http

import (
	nethttp "net/http"

	"audit-go/internal/delivery/http/middleware"
)

// RegisterRoutes wires all HTTP routes and returns the configured mux.
func RegisterRoutes(dep Dependencies) *nethttp.ServeMux {
	mux := nethttp.NewServeMux()

	// ── Public ────────────────────────────────────────────────────────────────

	mux.HandleFunc("GET /health", NewHandler(
		dep.Log,
		dep.CreateDocument,
		dep.DeleteDocument,
		dep.GetDocument,
	).Health)

	authH := NewAuthHandler(dep.Log, dep.Config, dep.Login, dep.Logout)
	mux.HandleFunc("POST /auth/login", authH.Login)

	// Logout behind auth so we always have an identity for auditing.
	auth := middleware.AuthWithRefresh(dep.Config, dep.JWT, dep.UserRepo, dep.RefreshRepo)
	mux.Handle("POST /auth/logout", auth(nethttp.HandlerFunc(authH.Logout)))

	// ── Authenticated ─────────────────────────────────────────────────────────

	h := NewHandler(dep.Log, dep.CreateDocument, dep.DeleteDocument, dep.GetDocument)

	mux.Handle("POST /documents", auth(nethttp.HandlerFunc(h.CreateDocument)))
	mux.Handle("GET /documents/get", auth(nethttp.HandlerFunc(h.GetDocument)))
	mux.Handle("DELETE /documents/delete", auth(nethttp.HandlerFunc(h.DeleteDocument)))

	return mux
}
