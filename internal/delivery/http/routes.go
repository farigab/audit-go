package http

import (
	nethttp "net/http"

	"audit-go/internal/delivery/http/middleware"
)

func RegisterRoutes(dep Dependencies) *nethttp.ServeMux {
	h := NewHandler(
		dep.Log,
		dep.CreateDocument,
		dep.DeleteDocument,
		dep.GetDocument,
	)

	mux := nethttp.NewServeMux()

	mux.HandleFunc("GET /health", h.Health)

	auth := middleware.AuthWithRefresh(
		dep.Config,
		dep.JWT,
		dep.UserRepo,
		dep.RefreshRepo,
	)

	mux.Handle(
		"POST /documents",
		auth(nethttp.HandlerFunc(h.CreateDocument)),
	)

	mux.Handle(
		"GET /documents/get",
		auth(nethttp.HandlerFunc(h.GetDocument)),
	)

	mux.Handle(
		"DELETE /documents/delete",
		auth(nethttp.HandlerFunc(h.DeleteDocument)),
	)

	return mux
}
