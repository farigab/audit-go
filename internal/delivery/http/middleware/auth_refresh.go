package middleware

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"

	"github.com/google/uuid"

	"audit-go/internal/cookies"
	"audit-go/internal/domain"
	"audit-go/internal/platform/config"
	"audit-go/internal/platform/security"
	"audit-go/internal/repository"
)

// AuthWithRefresh validates JWT cookie.
// If expired/invalid, tries refreshToken rotation.
func AuthWithRefresh(
	cfg *config.CookieConfig,
	jwtSvc security.TokenService,
	userRepo repository.UserRepository,
	refreshRepo repository.RefreshTokenRepository,
) func(http.Handler) http.Handler {
	// For the recommended flow we expect the access token in the
	// Authorization header (Bearer). Do NOT perform automatic rotation
	// here; require the client to call POST /auth/refresh explicitly.
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if userLogin := extractValidLogin(jwtSvc, r); userLogin != "" {
				ctx := context.WithValue(r.Context(), UserLoginKey, userLogin)
				next.ServeHTTP(w, r.WithContext(ctx))
				return
			}

			http.Error(w, "unauthorized", http.StatusUnauthorized)
		})
	}
}

func RotateRefreshToken(
	cfg *config.CookieConfig,
	jwtSvc security.TokenService,
	userRepo repository.UserRepository,
	refreshRepo repository.RefreshTokenRepository,
	w http.ResponseWriter,
	r *http.Request,
) (string, string, error) {
	if origin := r.Header.Get("Origin"); origin != "" {
		u, err := url.Parse(origin)
		if err != nil {
			cookies.ClearAuth(w, cfg)
			http.Error(w, "forbidden", http.StatusForbidden)
			return "", "", fmt.Errorf("invalid origin")
		}

		// Determine the effective request host. Prefer X-Forwarded-Host when
		// provided (common behind proxies). Take only the first value when a
		// comma-separated list is present. Then strip any port so we compare
		// hostnames only.
		reqHost := r.Header.Get("X-Forwarded-Host")
		if reqHost == "" {
			reqHost = r.Host
		}
		if i := strings.Index(reqHost, ","); i != -1 {
			reqHost = strings.TrimSpace(reqHost[:i])
		}
		reqHostname := reqHost
		if h, _, serr := net.SplitHostPort(reqHost); serr == nil {
			reqHostname = h
		}

		originHostname := u.Hostname()

		// If ALLOWED_ORIGINS is explicitly set, treat it as a comma-separated
		// allowlist of origins (hostnames or full URLs). Otherwise fall back
		// to the hostname equality check.
		allowed := strings.TrimSpace(os.Getenv("ALLOWED_ORIGINS"))
		if allowed != "" {
			ok := false
			for _, v := range strings.Split(allowed, ",") {
				v = strings.TrimSpace(v)
				if v == "" {
					continue
				}
				// Accept either full URL or host[:port]
				var aHost string
				if strings.Contains(v, "://") {
					if ua, perr := url.Parse(v); perr == nil {
						aHost = ua.Hostname()
					} else {
						aHost = v
					}
				} else {
					aHost = v
				}
				if strings.Contains(aHost, ":") {
					aHost = strings.Split(aHost, ":")[0]
				}
				if aHost == originHostname {
					ok = true
					break
				}
			}
			if !ok {
				cookies.ClearAuth(w, cfg)
				http.Error(w, "forbidden", http.StatusForbidden)
				return "", "", fmt.Errorf("origin not allowed")
			}
		} else {
			if originHostname == "" || originHostname != reqHostname {
				cookies.ClearAuth(w, cfg)
				http.Error(w, "forbidden", http.StatusForbidden)
				return "", "", fmt.Errorf("origin mismatch")
			}
		}
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

	// Detect reuse: if token already revoked then this value was used before.
	if oldRt.Revoked {
		// Revoke all sessions for this user as a safety measure.
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

	newToken := uuid.New().String()

	newRt := domain.NewRefreshToken(
		newToken,
		user.Login,
		time.Now().Add(7*24*time.Hour),
	)

	// Rotate (mark old revoked, insert new) in the repository.
	if err = refreshRepo.Rotate(r.Context(), oldRt, newRt); err != nil {
		http.Error(w, "failed to rotate refresh token", http.StatusBadGateway)
		return "", "", err
	}

	// Only set the refresh cookie. The new access token is returned in the
	// response body so the frontend can store it in memory and send it in
	// the Authorization header.
	cookies.SetWithPath(w, "refreshToken", newToken, 7*24*60*60, cfg, "/auth/refresh")

	return user.Login, jwtToken, nil
}
