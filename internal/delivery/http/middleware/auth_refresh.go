package middleware

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/google/uuid"

	"audit-go/internal/cookies"
	"audit-go/internal/domain"
	"audit-go/internal/platform/config"
	"audit-go/internal/platform/contextx"
	"audit-go/internal/platform/security"
	"audit-go/internal/repository"
)

// AuthWithRefresh validates the Bearer JWT.
// Actual token rotation is handled by POST /auth/refresh — this middleware
// does NOT rotate automatically.
func AuthWithRefresh(
	cfg *config.CookieConfig,
	jwtSvc security.TokenService,
	userRepo repository.UserRepository,
	refreshRepo repository.RefreshTokenRepository,
) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			userLogin := extractValidLogin(jwtSvc, r)
			if userLogin == "" {
				http.Error(w, "unauthorized", http.StatusUnauthorized)
				return
			}

			// Set both keys so handlers and audit use cases always find the identity,
			// matching the behaviour of the Auth middleware.
			ctx := context.WithValue(r.Context(), UserLoginKey, userLogin)
			ctx = contextx.Set(ctx, contextx.UserIDKey, userLogin)
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

// RotateRefreshToken validates the incoming refresh token cookie, rotates it,
// issues a new access token, and writes the new refresh token cookie.
// Returns (userLogin, newAccessToken, error).
func RotateRefreshToken(
	cfg *config.CookieConfig,
	jwtSvc security.TokenService,
	userRepo repository.UserRepository,
	refreshRepo repository.RefreshTokenRepository,
	w http.ResponseWriter,
	r *http.Request,
) (string, string, error) {
	if err := checkOrigin(cfg, w, r); err != nil {
		return "", "", err
	}

	rtCookie, err := r.Cookie("refreshToken")
	if err != nil || rtCookie.Value == "" {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return "", "", fmt.Errorf("missing refresh token")
	}

	oldRt, err := refreshRepo.FindByToken(r.Context(), rtCookie.Value)
	if err != nil {
		cookies.ClearAuth(w, cfg)
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return "", "", err
	}

	// Detect reuse: revoked tokens were already rotated — revoke all sessions.
	if oldRt.Revoked {
		if tokens, terr := refreshRepo.FindByUserLogin(r.Context(), oldRt.UserLogin); terr == nil {
			for _, t := range tokens {
				_ = refreshRepo.Delete(r.Context(), t)
			}
		}
		cookies.ClearAuth(w, cfg)
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return "", "", fmt.Errorf("refresh token reuse detected")
	}

	if time.Now().After(oldRt.ExpiresAt) {
		cookies.ClearAuth(w, cfg)
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return "", "", fmt.Errorf("refresh token expired")
	}

	user, err := userRepo.FindByLogin(r.Context(), oldRt.UserLogin)
	if err != nil {
		cookies.ClearAuth(w, cfg)
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return "", "", err
	}

	jwtToken, err := jwtSvc.GenerateToken(user.Login, map[string]interface{}{
		"name": user.Name,
	})
	if err != nil {
		http.Error(w, "failed to generate token", http.StatusBadGateway)
		return "", "", err
	}

	newRt := domain.NewRefreshToken(
		uuid.New().String(),
		user.Login,
		time.Now().Add(7*24*time.Hour),
	)

	if err = refreshRepo.Rotate(r.Context(), oldRt, newRt); err != nil {
		http.Error(w, "failed to rotate refresh token", http.StatusBadGateway)
		return "", "", err
	}

	// Access token goes in response body (stored in memory by the client).
	// Refresh token is HttpOnly, scoped to /auth/refresh only.
	cookies.SetWithPath(w, "refreshToken", newRt.Token, 7*24*60*60, cfg, "/auth/refresh")

	return user.Login, jwtToken, nil
}

// checkOrigin validates the Origin header against the configured allowlist.
// When ALLOWED_ORIGINS is empty, falls back to same-host comparison.
// Non-browser clients (no Origin header) always pass through.
func checkOrigin(cfg *config.CookieConfig, w http.ResponseWriter, r *http.Request) error {
	origin := r.Header.Get("Origin")
	if origin == "" {
		return nil
	}

	u, err := url.Parse(origin)
	if err != nil {
		cookies.ClearAuth(w, cfg)
		http.Error(w, "forbidden", http.StatusForbidden)
		return fmt.Errorf("invalid origin header")
	}

	if !originAllowed(u.Hostname(), strings.TrimSpace(cfg.AllowedOrigins), r) {
		cookies.ClearAuth(w, cfg)
		http.Error(w, "forbidden", http.StatusForbidden)
		return fmt.Errorf("origin not allowed")
	}

	return nil
}

// originAllowed is a pure decision function with no HTTP side effects.
// When allowlist is set it delegates to containsHostname; otherwise it
// compares origin against the effective request host.
func originAllowed(originHostname, allowlist string, r *http.Request) bool {
	if originHostname == "" {
		return false
	}
	if allowlist != "" {
		return containsHostname(allowlist, originHostname)
	}
	return originHostname == requestHostname(r)
}

// containsHostname reports whether target appears in the comma-separated allowlist.
// Each entry may be a full URL (https://example.com) or a bare host[:port].
func containsHostname(allowlist, target string) bool {
	for _, entry := range strings.Split(allowlist, ",") {
		if extractHostname(strings.TrimSpace(entry)) == target {
			return true
		}
	}
	return false
}

// extractHostname normalises a raw allowlist entry to a bare hostname.
// Accepts full URLs ("https://example.com:8080") or bare hosts ("example.com:8080").
func extractHostname(raw string) string {
	if raw == "" {
		return ""
	}
	if strings.Contains(raw, "://") {
		if u, err := url.Parse(raw); err == nil {
			return u.Hostname()
		}
		return raw
	}
	// bare host[:port] — strip optional port
	if host, _, err := net.SplitHostPort(raw); err == nil {
		return host
	}
	return raw
}

// requestHostname returns the effective hostname of the incoming request,
// preferring X-Forwarded-Host (set by proxies) over the Host header.
// Takes only the first value when a comma-separated list is present.
func requestHostname(r *http.Request) string {
	host := r.Header.Get("X-Forwarded-Host")
	if host == "" {
		host = r.Host
	}
	if i := strings.Index(host, ","); i != -1 {
		host = strings.TrimSpace(host[:i])
	}
	if h, _, err := net.SplitHostPort(host); err == nil {
		return h
	}
	return host
}
