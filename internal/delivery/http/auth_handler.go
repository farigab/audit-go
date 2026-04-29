package http

import (
	"encoding/json"
	"net/http"

	"github.com/rs/zerolog"

	"audit-go/internal/cookies"
	"audit-go/internal/platform/config"
	"audit-go/internal/usecase"
)

// AuthHandler handles authentication endpoints.
type AuthHandler struct {
	log    zerolog.Logger
	cfg    *config.CookieConfig
	login  usecase.LoginUseCase
	logout usecase.LogoutUseCase
	refresh usecase.RefreshUseCase
}

// NewAuthHandler creates a new AuthHandler.
func NewAuthHandler(
	log zerolog.Logger,
	cfg *config.CookieConfig,
	login usecase.LoginUseCase,
	logout usecase.LogoutUseCase,
	refresh usecase.RefreshUseCase,
) AuthHandler {
	return AuthHandler{
		log:     log,
		cfg:     cfg,
		login:   login,
		logout:  logout,
		refresh: refresh,
	}
}

// Login handles POST /auth/login.
// Validates credentials, sets the refresh token as an HttpOnly cookie scoped
// to /auth/refresh, and returns the access token + basic user info in the body.
// The access token is kept in memory by the frontend — never in a cookie or
// localStorage — to avoid CSRF and XSS exposure respectively.
func (h AuthHandler) Login(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Login string `json:"login"`
	}

	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		h.writeError(w, r, http.StatusBadRequest, "invalid request body")
		return
	}

	out, err := h.login.Execute(r.Context(), usecase.LoginInput{Login: body.Login})
	if err != nil {
		// Always 401 — do not reveal whether the login exists.
		h.writeError(w, r, http.StatusUnauthorized, "invalid credentials")
		return
	}

	// Scope the refresh token cookie to the refresh endpoint so it is never
	// sent on API requests — limits the window for token theft via XSS.
	cookies.SetWithPath(w, "refreshToken", out.RefreshToken, 7*24*60*60, h.cfg, "/auth/refresh")

	h.writeJSON(w, r, http.StatusOK, map[string]string{
		"login":       out.User.Login,
		"name":        out.User.Name,
		"accessToken": out.AccessToken,
	})
}

// Refresh handles POST /auth/refresh.
// Rotates the refresh token and returns a new access token.
// The new refresh token is set as an HttpOnly cookie; the new access token is
// returned in the response body.
func (h AuthHandler) Refresh(w http.ResponseWriter, r *http.Request) {
	rtCookie, err := r.Cookie("refreshToken")
	if err != nil || rtCookie.Value == "" {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}

	out, err := h.refresh.Execute(r.Context(), usecase.RefreshInput{
		RawToken: rtCookie.Value,
	})
	if err != nil {
		h.log.Warn().Err(err).Msg("refresh: failed")
		cookies.ClearAuth(w, h.cfg)
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}

	cookies.SetWithPath(w, "refreshToken", out.NewRefreshToken, 7*24*60*60, h.cfg, "/auth/refresh")

	h.writeJSON(w, r, http.StatusOK, map[string]string{
		"login":       out.User.Login,
		"name":        out.User.Name,
		"accessToken": out.AccessToken,
	})
}

// Logout handles POST /auth/logout.
// Revokes the refresh token and clears auth cookies. Idempotent — a missing
// or already-revoked token is treated as success so the client always ends up
// with cleared cookies.
func (h AuthHandler) Logout(w http.ResponseWriter, r *http.Request) {
	rawToken := ""
	if c, err := r.Cookie("refreshToken"); err == nil {
		rawToken = c.Value
	}

	if err := h.logout.Execute(r.Context(), rawToken); err != nil {
		h.log.Warn().Err(err).Msg("logout: failed to revoke refresh token")
	}

	cookies.ClearAuth(w, h.cfg)

	h.writeJSON(w, r, http.StatusOK, map[string]string{"status": "logged out"})
}

func (h AuthHandler) writeJSON(w http.ResponseWriter, r *http.Request, status int, data any) {
	if err := WriteJSON(w, status, data); err != nil {
		h.log.Error().Err(err).Str("path", r.URL.Path).Msg("failed to write response")
	}
}

func (h AuthHandler) writeError(w http.ResponseWriter, r *http.Request, status int, msg string) {
	if err := WriteError(w, status, msg); err != nil {
		h.log.Error().Err(err).Str("path", r.URL.Path).Msg("failed to write error response")
	}
}
