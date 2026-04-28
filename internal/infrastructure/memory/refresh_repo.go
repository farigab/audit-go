// Package memory provides simple in-memory implementations used for local development and tests.
package memory

import (
	"context"
	"fmt"
	"sync"
	"time"
)

// RefreshToken represents a refresh token persisted in memory.
type RefreshToken struct {
	Token     string
	UserLogin string
	ExpiresAt time.Time
	CreatedAt time.Time
	Revoked   bool
}

// RefreshRepo stores refresh tokens in memory. Not safe for production.
type RefreshRepo struct {
	mu     sync.Mutex
	tokens map[string]*RefreshToken
}

// NewRefreshTokenRepo creates an in-memory refresh token repository.
func NewRefreshTokenRepo() *RefreshRepo {
	return &RefreshRepo{tokens: make(map[string]*RefreshToken)}
}

// Save persists a refresh token in memory.
func (r *RefreshRepo) Save(ctx context.Context, t *RefreshToken) (*RefreshToken, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if t == nil {
		return nil, nil
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	r.tokens[t.Token] = t
	return t, nil
}

// FindByToken returns a refresh token by its token string.
func (r *RefreshRepo) FindByToken(ctx context.Context, token string) (*RefreshToken, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	t, ok := r.tokens[token]
	if !ok {
		return nil, fmt.Errorf("refresh token not found")
	}
	return t, nil
}

// FindByUserLogin returns all refresh tokens for a given user.
func (r *RefreshRepo) FindByUserLogin(ctx context.Context, userLogin string) ([]*RefreshToken, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	var list []*RefreshToken
	for _, t := range r.tokens {
		if t.UserLogin == userLogin {
			list = append(list, t)
		}
	}
	return list, nil
}

// Delete removes a refresh token.
func (r *RefreshRepo) Delete(ctx context.Context, t *RefreshToken) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if t == nil {
		return nil
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	delete(r.tokens, t.Token)
	return nil
}

// DeleteAllByUserLogin revokes all tokens for a given user login.
func (r *RefreshRepo) DeleteAllByUserLogin(ctx context.Context, userLogin string) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	for k, t := range r.tokens {
		if t.UserLogin == userLogin {
			delete(r.tokens, k)
		}
	}
	return nil
}

// DeleteExpiredTokens removes tokens that have expired.
func (r *RefreshRepo) DeleteExpiredTokens(ctx context.Context) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	now := time.Now()
	r.mu.Lock()
	defer r.mu.Unlock()
	for k, t := range r.tokens {
		if t.ExpiresAt.Before(now) {
			delete(r.tokens, k)
		}
	}
	return nil
}
