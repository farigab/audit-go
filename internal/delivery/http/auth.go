package http

import (
	"net/http"
	"time"

	"github.com/google/uuid"

	"audit-go/internal/infrastructure/memory"
	"audit-go/internal/platform/contextx"
)

// RegisterAuthRoutes registers minimal auth routes (no GitHub flow).
// Routes:
// - POST /api/auth/refresh : rotate refresh token and issue a new access token cookie
// - POST /api/auth/logout  : revoke refresh tokens and clear cookies
func RegisterAuthRoutes(mux *http.ServeMux, repo *memory.RefreshRepo) {
	mux.HandleFunc("/api/auth/refresh", func(w http.ResponseWriter, r *http.Request) {
		handleRefresh(w, r, repo)
	})
	mux.HandleFunc("/api/auth/logout", func(w http.ResponseWriter, r *http.Request) {
		handleLogout(w, r, repo)
	})
}

func handleRefresh(w http.ResponseWriter, r *http.Request, repo *memory.RefreshRepo) {
	cookie, err := r.Cookie("refreshToken")
	if err != nil || cookie.Value == "" {
		clearAuthCookies(w)
		http.Error(w, "no refresh token", http.StatusUnauthorized)
		return
	}

	oldRt, err := repo.FindByToken(r.Context(), cookie.Value)
	if err != nil {
		clearAuthCookies(w)
		http.Error(w, "invalid refresh token", http.StatusUnauthorized)
		return
	}

	if oldRt.Revoked || time.Now().After(oldRt.ExpiresAt) {
		clearAuthCookies(w)
		http.Error(w, "refresh token expired", http.StatusUnauthorized)
		return
	}

	user := oldRt.UserLogin

	// Generate a simple opaque access token (UUID). Replace with real JWT later.
	accessToken := uuid.NewString()

	// Create new refresh token and persist it.
	newToken := uuid.NewString()
	rt := &memory.RefreshToken{
		Token:     newToken,
		UserLogin: user,
		ExpiresAt: time.Now().Add(7 * 24 * time.Hour),
		CreatedAt: time.Now(),
		Revoked:   false,
	}
	if _, err := repo.Save(r.Context(), rt); err != nil {
		http.Error(w, "failed to save refresh token", http.StatusInternalServerError)
		return
	}

	_ = repo.Delete(r.Context(), oldRt)

	setCookie(w, "token", accessToken, 15*60)
	setCookie(w, "refreshToken", newToken, 7*24*60*60)

	w.WriteHeader(http.StatusOK)
}

func handleLogout(w http.ResponseWriter, r *http.Request, repo *memory.RefreshRepo) {
	// Prefer explicit user from context (set by RequestContext middleware)
	if login := contextx.Get(r.Context(), contextx.UserIDKey); login != "" {
		_ = repo.DeleteAllByUserLogin(r.Context(), login)
	} else if rt, err := r.Cookie("refreshToken"); err == nil && rt.Value != "" {
		if found, err := repo.FindByToken(r.Context(), rt.Value); err == nil {
			_ = repo.Delete(r.Context(), found)
		}
	}

	clearAuthCookies(w)
	w.WriteHeader(http.StatusOK)
}

func setCookie(w http.ResponseWriter, name, value string, maxAge int) {
	c := &http.Cookie{
		Name:     name,
		Value:    value,
		HttpOnly: true,
		Path:     "/",
		MaxAge:   maxAge,
	}
	http.SetCookie(w, c)
}

func clearAuthCookies(w http.ResponseWriter) {
	// delete cookies by setting MaxAge=-1
	setCookie(w, "token", "", -1)
	setCookie(w, "refreshToken", "", -1)
}
