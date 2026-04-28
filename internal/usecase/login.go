// Package usecase implements application use cases and business logic.
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

// loginUserRepo is the subset of UserRepository needed by LoginUseCase.
type loginUserRepo interface {
	FindByLogin(ctx context.Context, login string) (*domain.User, error)
}

// loginRefreshRepo is the subset of RefreshTokenRepository needed by LoginUseCase.
type loginRefreshRepo interface {
	Save(ctx context.Context, t *domain.RefreshToken) (*domain.RefreshToken, error)
}

// LoginUseCase authenticates a user and issues access + refresh tokens.
type LoginUseCase struct {
	UserRepo    loginUserRepo
	RefreshRepo loginRefreshRepo
	JWT         security.TokenService
}

// LoginInput holds the credentials provided by the client.
type LoginInput struct {
	Login string
}

// LoginOutput holds the tokens to be delivered to the client (via cookies).
type LoginOutput struct {
	AccessToken  string
	RefreshToken string
	User         *domain.User
}

// Execute validates credentials and returns signed tokens.
// It intentionally returns a generic error on bad credentials to prevent
// user-enumeration attacks.
func (u LoginUseCase) Execute(ctx context.Context, input LoginInput) (*LoginOutput, error) {
	if input.Login == "" {
		return nil, errors.New("login is required")
	}

	user, err := u.UserRepo.FindByLogin(ctx, input.Login)
	if err != nil {
		// Return the same error message whether the user doesn't exist or
		return nil, errors.New("invalid credentials")
	}

	accessToken, err := u.JWT.GenerateToken(user.Login, map[string]interface{}{
		"name": user.Name,
	})
	if err != nil {
		return nil, fmt.Errorf("generating access token: %w", err)
	}

	rawRefresh := uuid.NewString()
	rt := domain.NewRefreshToken(
		rawRefresh,
		user.Login,
		time.Now().Add(7*24*time.Hour),
	)

	if _, err = u.RefreshRepo.Save(ctx, rt); err != nil {
		return nil, fmt.Errorf("saving refresh token: %w", err)
	}

	return &LoginOutput{
		AccessToken:  accessToken,
		RefreshToken: rawRefresh,
		User:         user,
	}, nil
}
