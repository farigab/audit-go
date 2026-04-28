// Package repository contains repository implementations used by the application.
package repository

import (
	"context"

	"audit-go/internal/domain"
)

// RefreshTokenRepository defines operations for refresh tokens.
type RefreshTokenRepository interface {
	Save(ctx context.Context, t *domain.RefreshToken) (*domain.RefreshToken, error)
	FindByToken(ctx context.Context, token string) (*domain.RefreshToken, error)
	FindByUserLogin(ctx context.Context, userLogin string) ([]*domain.RefreshToken, error)

	Rotate(ctx context.Context, old, new *domain.RefreshToken) error

	Delete(ctx context.Context, t *domain.RefreshToken) error
	DeleteExpiredTokens(ctx context.Context) error
}
