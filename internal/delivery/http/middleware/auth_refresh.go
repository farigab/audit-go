package middleware

import (
	"context"
	"fmt"
	"net/http"
	"time"

	"github.com/google/uuid"

	"audit-go/internal/cookies"
	"audit-go/internal/domain"
	"audit-go/internal/platform/config"
	"audit-go/internal/platform/contextx"
	"audit-go/internal/platform/origin"
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

	rawNewToken := uuid.New().String()
	newRt := domain.NewRefreshToken(
		rawNewToken,
		user.Login,
		time.Now().Add(7*24*time.Hour),
	)

	if err = refreshRepo.Rotate(r.Context(), oldRt, newRt); err != nil {
		http.Error(w, "failed to rotate refresh token", http.StatusBadGateway)
		return "", "", err
	}

	cookies.SetWithPath(w, "refreshToken", rawNewToken, 7*24*60*60, cfg, "/auth/refresh")

	return user.Login, jwtToken, nil
}

// checkOrigin valida o header Origin usando origin.Allowlist.
//
// Três casos:
//  1. Origin ausente  → passa (request não-browser, ex: curl, mobile nativo).
//  2. Allowlist vazia → bloqueia qualquer Origin presente. Mais seguro do que
//     o fallback anterior que comparava com o hostname do request.
//  3. Origin presente → deve estar na allowlist.
//
// A Allowlist é construída por request aqui porque /auth/refresh tem baixa
// frequência. Se isso mudar, injete a Allowlist pré-construída via closure.
func checkOrigin(cfg *config.CookieConfig, w http.ResponseWriter, r *http.Request) error {
	originHeader := r.Header.Get("Origin")
	if originHeader == "" {
		return nil
	}

	al := origin.Parse(cfg.AllowedOrigins)

	if !al.Allows(originHeader) {
		cookies.ClearAuth(w, cfg)
		http.Error(w, "forbidden", http.StatusForbidden)
		return fmt.Errorf("origin not allowed: %s", originHeader)
	}

	return nil
}
