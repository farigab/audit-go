// Package http exposes access and authentication HTTP handlers.
package http

import (
	"net/http"
	"time"

	"github.com/rs/zerolog"

	"audit-go/internal/features/access"
	accessapp "audit-go/internal/features/access/app"
	"audit-go/internal/platform/config"
	"audit-go/internal/platform/httpx"
)

const (
	SessionCookie = "audit_session"
	RefreshCookie = "audit_refresh"
	CSRFCookie    = "audit_csrf"
	CSRFHeader    = "X-CSRF-Token"
)

// Handler handles auth endpoints.
type Handler struct {
	log     zerolog.Logger
	service *accessapp.Service
	cookies *config.CookieConfig
}

// NewHandler creates an auth handler.
func NewHandler(log zerolog.Logger, service *accessapp.Service, cookies *config.CookieConfig) Handler {
	return Handler{log: log, service: service, cookies: cookies}
}

// RegisterRoutes wires auth routes.
func RegisterRoutes(mux *http.ServeMux, auth func(http.Handler) http.Handler, h Handler) {
	mux.HandleFunc("GET /auth/login", h.Login)
	mux.HandleFunc("GET /auth/callback", h.Callback)
	mux.HandleFunc("POST /auth/refresh", h.Refresh)
	mux.HandleFunc("POST /auth/logout", h.Logout)
	mux.Handle("GET /auth/me", auth(http.HandlerFunc(h.Me)))
}

// Login redirects the browser to Microsoft Entra.
func (h Handler) Login(w http.ResponseWriter, r *http.Request) {
	loginURL, err := h.service.LoginURL(r.Context(), r.URL.Query().Get("return_url"))
	if err != nil {
		h.log.Error().Err(err).Msg("failed to create login URL")
		h.writeError(w, r, http.StatusInternalServerError, "failed to start login")
		return
	}

	http.Redirect(w, r, loginURL, http.StatusFound)
}

// Callback completes the Entra authorization code flow and issues app cookies.
func (h Handler) Callback(w http.ResponseWriter, r *http.Request) {
	result, err := h.service.CompleteCallback(
		r.Context(),
		r.URL.Query().Get("code"),
		r.URL.Query().Get("state"),
	)
	if err != nil {
		h.log.Error().Err(err).Msg("failed to complete login callback")
		h.writeError(w, r, http.StatusUnauthorized, "login failed")
		return
	}

	h.writeAuthCookies(w, result)
	http.Redirect(w, r, result.ReturnURL, http.StatusFound)
}

// Refresh rotates the refresh token and issues a new app session.
func (h Handler) Refresh(w http.ResponseWriter, r *http.Request) {
	refresh, err := r.Cookie(RefreshCookie)
	if err != nil || refresh.Value == "" {
		h.writeError(w, r, http.StatusUnauthorized, "missing refresh token")
		return
	}

	result, err := h.service.Refresh(r.Context(), refresh.Value)
	if err != nil {
		h.clearAuthCookies(w)
		h.writeError(w, r, http.StatusUnauthorized, "invalid refresh token")
		return
	}

	h.writeAuthCookies(w, result)
	if err = httpx.WriteJSON(w, http.StatusOK, map[string]any{
		"user": result.Principal,
	}); err != nil {
		h.logWriteError(r, err)
	}
}

// Logout revokes the current app session and refresh token.
func (h Handler) Logout(w http.ResponseWriter, r *http.Request) {
	var sessionValue string
	if c, err := r.Cookie(SessionCookie); err == nil {
		sessionValue = c.Value
	}
	var refreshValue string
	if c, err := r.Cookie(RefreshCookie); err == nil {
		refreshValue = c.Value
	}

	if err := h.service.Logout(r.Context(), sessionValue, refreshValue); err != nil {
		h.log.Error().Err(err).Msg("failed to revoke auth cookies")
	}
	h.clearAuthCookies(w)

	if err := httpx.WriteJSON(w, http.StatusOK, map[string]string{"status": "logged_out"}); err != nil {
		h.logWriteError(r, err)
	}
}

// Me returns the current authenticated principal.
func (h Handler) Me(w http.ResponseWriter, r *http.Request) {
	principal, ok := access.PrincipalFromContext(r.Context())
	if !ok {
		h.writeError(w, r, http.StatusUnauthorized, "unauthorized")
		return
	}

	if err := httpx.WriteJSON(w, http.StatusOK, principal); err != nil {
		h.logWriteError(r, err)
	}
}

func (h Handler) writeAuthCookies(w http.ResponseWriter, result *accessapp.AuthResult) {
	setCookie(w, h.cookies, cookieSpec{
		Name:     SessionCookie,
		Value:    result.SessionToken,
		Path:     "/",
		Expires:  result.SessionExpiresAt,
		MaxAge:   secondsUntil(result.SessionExpiresAt),
		HttpOnly: true,
	})
	setCookie(w, h.cookies, cookieSpec{
		Name:     RefreshCookie,
		Value:    result.RefreshToken,
		Path:     "/auth",
		Expires:  result.RefreshExpiresAt,
		MaxAge:   secondsUntil(result.RefreshExpiresAt),
		HttpOnly: true,
	})
	setCookie(w, h.cookies, cookieSpec{
		Name:     CSRFCookie,
		Value:    result.CSRFToken,
		Path:     "/",
		Expires:  result.RefreshExpiresAt,
		MaxAge:   secondsUntil(result.RefreshExpiresAt),
		HttpOnly: false,
	})
}

func (h Handler) clearAuthCookies(w http.ResponseWriter) {
	for _, spec := range []cookieSpec{
		{Name: SessionCookie, Path: "/", MaxAge: -1, HttpOnly: true},
		{Name: RefreshCookie, Path: "/auth", MaxAge: -1, HttpOnly: true},
		{Name: CSRFCookie, Path: "/", MaxAge: -1, HttpOnly: false},
	} {
		setCookie(w, h.cookies, spec)
	}
}

func (h Handler) writeError(w http.ResponseWriter, r *http.Request, status int, message string) {
	if err := httpx.WriteError(w, status, message); err != nil {
		h.logWriteError(r, err)
	}
}

func (h Handler) logWriteError(r *http.Request, err error) {
	h.log.Error().
		Err(err).
		Str("path", r.URL.Path).
		Msg("failed to write response")
}

type cookieSpec struct {
	Name     string
	Value    string
	Path     string
	Expires  time.Time
	MaxAge   int
	HttpOnly bool
}

func setCookie(w http.ResponseWriter, cfg *config.CookieConfig, spec cookieSpec) {
	secure := true
	sameSite := http.SameSiteLaxMode
	var domain string
	if cfg != nil {
		secure = cfg.CookieSecure
		sameSite = parseSameSite(cfg.CookieSameSite)
		domain = cfg.CookieDomain
	}

	c := &http.Cookie{
		Name:     spec.Name,
		Value:    spec.Value,
		Path:     spec.Path,
		Expires:  spec.Expires,
		MaxAge:   spec.MaxAge,
		HttpOnly: spec.HttpOnly,
		Secure:   secure,
		SameSite: sameSite,
	}
	if domain != "" && domain != "localhost" {
		c.Domain = domain
	}

	http.SetCookie(w, c)
}

func parseSameSite(s string) http.SameSite {
	switch s {
	case "Strict":
		return http.SameSiteStrictMode
	case "None":
		return http.SameSiteNoneMode
	default:
		return http.SameSiteLaxMode
	}
}

func secondsUntil(t time.Time) int {
	if t.IsZero() {
		return 0
	}
	seconds := int(time.Until(t).Seconds())
	if seconds < 1 {
		return 1
	}
	return seconds
}
