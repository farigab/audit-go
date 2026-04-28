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
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if userLogin := extractValidLogin(jwtSvc, r); userLogin != "" {
				ctx := context.WithValue(r.Context(), UserLoginKey, userLogin)
				next.ServeHTTP(w, r.WithContext(ctx))
				return
			}

			userLogin, err := rotateRefreshToken(
				cfg,
				jwtSvc,
				userRepo,
				refreshRepo,
				w,
				r,
			)
			if err != nil {
				return
			}

			ctx := context.WithValue(r.Context(), UserLoginKey, userLogin)
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

func rotateRefreshToken(
	cfg *config.CookieConfig,
	jwtSvc security.TokenService,
	userRepo repository.UserRepository,
	refreshRepo repository.RefreshTokenRepository,
	w http.ResponseWriter,
	r *http.Request,
) (string, error) {
	rtCookie, err := r.Cookie("refreshToken")
	if err != nil || rtCookie.Value == "" {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return "", fmt.Errorf("missing refresh token")
	}

	oldRt, err := refreshRepo.FindByToken(r.Context(), rtCookie.Value)
	if err != nil {
		cookies.ClearAuth(w, cfg)
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return "", err
	}

	if oldRt.Revoked || time.Now().After(oldRt.ExpiresAt) {
		cookies.ClearAuth(w, cfg)
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return "", fmt.Errorf("refresh token expired or revoked")
	}

	user, err := userRepo.FindByLogin(r.Context(), oldRt.UserLogin)
	if err != nil {
		cookies.ClearAuth(w, cfg)
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return "", err
	}

	jwtToken, err := jwtSvc.GenerateToken(user.Login, map[string]interface{}{
		"name":   user.Name,
		"tenant": "default",
	})
	if err != nil {
		http.Error(w, "failed to generate token", http.StatusBadGateway)
		return "", err
	}

	newToken := uuid.New().String()

	newRt := domain.NewRefreshToken(
		newToken,
		user.Login,
		time.Now().Add(7*24*time.Hour),
	)

	if _, err = refreshRepo.Save(r.Context(), newRt); err != nil {
		http.Error(w, "failed to save refresh token", http.StatusBadGateway)
		return "", err
	}

	if err = refreshRepo.Delete(r.Context(), oldRt); err != nil {
		http.Error(w, "failed to revoke old refresh token", http.StatusBadGateway)
		return "", err
	}

	cookies.Set(w, "token", jwtToken, 15*60, cfg)
	cookies.Set(w, "refreshToken", newToken, 7*24*60*60, cfg)

	return user.Login, nil
}
