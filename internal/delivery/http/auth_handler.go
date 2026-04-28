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
}

// NewAuthHandler creates a new AuthHandler.
func NewAuthHandler(
	log zerolog.Logger,
	cfg *config.CookieConfig,
	login usecase.LoginUseCase,
	logout usecase.LogoutUseCase,
) AuthHandler {
	return AuthHandler{
		log:    log,
		cfg:    cfg,
		login:  login,
		logout: logout,
	}
}

// Login handles POST /auth/login.
// It validates credentials, sets HttpOnly cookies, and returns basic user info.
func (h AuthHandler) Login(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Login string `json:"login"`
	}

	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		if werr := WriteError(w, http.StatusBadRequest, "invalid request body"); werr != nil {
			h.logWriteError(r, werr)
		}
		return
	}

	out, err := h.login.Execute(r.Context(), usecase.LoginInput{
		Login: body.Login,
	})
	if err != nil {
		// Always 401 — do not leak whether the login exists.
		if werr := WriteError(w, http.StatusUnauthorized, "invalid credentials"); werr != nil {
			h.logWriteError(r, werr)
		}
		return
	}

	// 15-minute access token, 7-day refresh token.
	cookies.Set(w, "token", out.AccessToken, 15*60, h.cfg)
	cookies.Set(w, "refreshToken", out.RefreshToken, 7*24*60*60, h.cfg)

	if err = WriteJSON(w, http.StatusOK, map[string]string{
		"login": out.User.Login,
		"name":  out.User.Name,
	}); err != nil {
		h.logWriteError(r, err)
	}
}

// Logout handles POST /auth/logout.
// It revokes the refresh token and clears auth cookies.
func (h AuthHandler) Logout(w http.ResponseWriter, r *http.Request) {
	rtCookie, err := r.Cookie("refreshToken")
	rawToken := ""
	if err == nil {
		rawToken = rtCookie.Value
	}

	// Best-effort revocation — ignore errors so the client always gets cookies cleared.
	if err = h.logout.Execute(r.Context(), rawToken); err != nil {
		h.log.Warn().Err(err).Msg("logout: failed to revoke refresh token")
	}

	cookies.ClearAuth(w, h.cfg)

	if err = WriteJSON(w, http.StatusOK, map[string]string{"status": "logged out"}); err != nil {
		h.logWriteError(r, err)
	}
}

func (h AuthHandler) logWriteError(r *http.Request, err error) {
	h.log.Error().
		Err(err).
		Str("path", r.URL.Path).
		Msg("failed to write response")
}
