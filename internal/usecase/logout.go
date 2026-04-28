package usecase

import (
	"context"
	"fmt"

	"audit-go/internal/domain"
)

// logoutRefreshRepo is the subset of RefreshTokenRepository needed by LogoutUseCase.
type logoutRefreshRepo interface {
	FindByToken(ctx context.Context, token string) (*domain.RefreshToken, error)
	Delete(ctx context.Context, t *domain.RefreshToken) error
}

// LogoutUseCase revokes the caller's refresh token.
type LogoutUseCase struct {
	RefreshRepo logoutRefreshRepo
}

// Execute deletes the provided refresh token, invalidating the session.
// It is intentionally a no-op when the token is not found so that double
// logouts are idempotent from the client's perspective.
func (u LogoutUseCase) Execute(ctx context.Context, rawToken string) error {
	if rawToken == "" {
		return nil
	}

	rt, err := u.RefreshRepo.FindByToken(ctx, rawToken)
	if err != nil {
		// Token not found — already logged out; treat as success.
		return nil
	}

	if err = u.RefreshRepo.Delete(ctx, rt); err != nil {
		return fmt.Errorf("revoking refresh token: %w", err)
	}

	return nil
}
