package http

import (
	nethttp "net/http"

	"audit-go/internal/delivery/http/middleware"
)

// RegisterRoutes wires all HTTP routes and returns the configured mux.
func RegisterRoutes(dep Dependencies) *nethttp.ServeMux {
	mux := nethttp.NewServeMux()

	h := NewHandler(dep.Log, dep.CreateDocument, dep.DeleteDocument, dep.GetDocument)
	auth := middleware.Auth(dep.Entra)

	// ── Public ────────────────────────────────────────────────────────────────
	// Authentication is handled entirely by Microsoft Entra ID (MSAL on the
	// frontend). The Go API only validates the resulting bearer token.

	mux.HandleFunc("GET /health", h.Health)

	// ── Authenticated ─────────────────────────────────────────────────────────

	mux.Handle("POST /documents", auth(nethttp.HandlerFunc(h.CreateDocument)))
	mux.Handle("GET /documents/get", auth(nethttp.HandlerFunc(h.GetDocument)))
	mux.Handle("DELETE /documents/delete", auth(nethttp.HandlerFunc(h.DeleteDocument)))

	return mux
}
