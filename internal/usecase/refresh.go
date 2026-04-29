package usecase

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"

	"audit-go/internal/domain"
	"audit-go/internal/platform/security"
)

// refreshUserRepo is the subset of UserRepository needed by RefreshUseCase.
type refreshUserRepo interface {
	FindByLogin(ctx context.Context, login string) (*domain.User, error)
}

// refreshTokenRepo is the subset of RefreshTokenRepository needed by RefreshUseCase.
type refreshTokenRepo interface {
	FindByToken(ctx context.Context, token string) (*domain.RefreshToken, error)
	FindByUserLogin(ctx context.Context, userLogin string) ([]*domain.RefreshToken, error)
	Rotate(ctx context.Context, old, next *domain.RefreshToken) error
	Delete(ctx context.Context, t *domain.RefreshToken) error
}

// RefreshUseCase rotates a refresh token and issues a new access token.
type RefreshUseCase struct {
	UserRepo    refreshUserRepo
	RefreshRepo refreshTokenRepo
	JWT         security.TokenService
}

// RefreshInput holds the raw refresh token provided by the caller.
type RefreshInput struct {
	RawToken string
}

// RefreshOutput holds the new tokens and the authenticated user.
type RefreshOutput struct {
	AccessToken     string
	NewRefreshToken string
	User            *domain.User
}

// Execute validates the incoming refresh token, detects reuse, rotates it,
// and returns a new access token and refresh token.
//
// On reuse detection (a previously revoked token is presented again), all
// sessions for that user are invalidated to limit the blast radius of a
// stolen token.
func (u RefreshUseCase) Execute(ctx context.Context, input RefreshInput) (*RefreshOutput, error) {
	if input.RawToken == "" {
		return nil, errors.New("refresh token is required")
	}

	oldRt, err := u.RefreshRepo.FindByToken(ctx, input.RawToken)
	if err != nil {
		return nil, errors.New("invalid refresh token")
	}

	// Reuse detection: a revoked token being presented again means a previous
	// rotation already happened. Invalidate all sessions for this user.
	if oldRt.Revoked {
		if tokens, terr := u.RefreshRepo.FindByUserLogin(ctx, oldRt.UserLogin); terr == nil {
			for _, t := range tokens {
				_ = u.RefreshRepo.Delete(ctx, t)
			}
		}
		return nil, errors.New("refresh token reuse detected: all sessions revoked")
	}

	if time.Now().After(oldRt.ExpiresAt) {
		return nil, errors.New("refresh token expired")
	}

	user, err := u.UserRepo.FindByLogin(ctx, oldRt.UserLogin)
	if err != nil {
		return nil, fmt.Errorf("user not found: %w", err)
	}

	accessToken, err := u.JWT.GenerateToken(user.Login, map[string]interface{}{
		"name": user.Name,
	})
	if err != nil {
		return nil, fmt.Errorf("generating access token: %w", err)
	}

	rawNew := uuid.New().String()
	newRt := domain.NewRefreshToken(
		rawNew,
		user.Login,
		time.Now().Add(7*24*time.Hour),
	)

	if err = u.RefreshRepo.Rotate(ctx, oldRt, newRt); err != nil {
		return nil, fmt.Errorf("rotating refresh token: %w", err)
	}

	return &RefreshOutput{
		AccessToken:     accessToken,
		NewRefreshToken: rawNew,
		User:            user,
	}, nil
}
