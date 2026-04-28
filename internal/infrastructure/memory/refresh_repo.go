package memory

import (
	"context"
	"fmt"
	"sync"
	"time"
)

type RefreshToken struct {
	Token     string
	UserLogin string
	ExpiresAt time.Time
	CreatedAt time.Time
	Revoked   bool
}

type RefreshRepo struct {
	mu     sync.Mutex
	tokens map[string]*RefreshToken
}

// NewRefreshTokenRepo creates an in-memory refresh token repository.
func NewRefreshTokenRepo() *RefreshRepo {
	return &RefreshRepo{tokens: make(map[string]*RefreshToken)}
}

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
