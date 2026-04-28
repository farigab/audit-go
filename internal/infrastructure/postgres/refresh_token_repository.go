// Package postgres provides PostgreSQL repository implementations.
package postgres

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"audit-go/internal/domain"
)

type RefreshTokenRepository struct {
	db *sql.DB
}

// NewRefreshTokenRepository creates a PostgreSQL refresh token repository.
func NewRefreshTokenRepository(db *sql.DB) *RefreshTokenRepository {
	return &RefreshTokenRepository{db: db}
}

// Save inserts a refresh token.
func (r *RefreshTokenRepository) Save(
	ctx context.Context,
	token *domain.RefreshToken,
) (*domain.RefreshToken, error) {
	const query = `
		INSERT INTO refresh_tokens (token, user_login, expires_at, revoked, created_at)
		VALUES ($1, $2, $3, $4, NOW())
	`

	_, err := r.db.ExecContext(ctx, query, token.Token, token.UserLogin, token.ExpiresAt, token.Revoked)
	if err != nil {
		return nil, fmt.Errorf("saving refresh token: %w", err)
	}

	return token, nil
}

func (r *RefreshTokenRepository) Rotate(
	ctx context.Context,
	old, new *domain.RefreshToken,
) error {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("beginning rotation transaction: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	res, err := tx.ExecContext(ctx, `DELETE FROM refresh_tokens WHERE token = $1`, old.Token)
	if err != nil {
		return fmt.Errorf("deleting old refresh token: %w", err)
	}
	if n, _ := res.RowsAffected(); n == 0 {
		// Token was already deleted (concurrent logout); treat as expired.
		return errors.New("refresh token not found or already revoked")
	}

	_, err = tx.ExecContext(ctx,
		`INSERT INTO refresh_tokens (token, user_login, expires_at, revoked, created_at) VALUES ($1, $2, $3, $4, NOW())`,
		new.Token, new.UserLogin, new.ExpiresAt, new.Revoked,
	)
	if err != nil {
		return fmt.Errorf("inserting new refresh token: %w", err)
	}

	if err = tx.Commit(); err != nil {
		return fmt.Errorf("committing rotation transaction: %w", err)
	}

	return nil
}

// FindByToken returns a refresh token by token value.
func (r *RefreshTokenRepository) FindByToken(
	ctx context.Context,
	value string,
) (*domain.RefreshToken, error) {
	const query = `
		SELECT token, user_login, expires_at, revoked
		FROM refresh_tokens
		WHERE token = $1
		LIMIT 1
	`

	var token domain.RefreshToken
	err := r.db.QueryRowContext(ctx, query, value).Scan(
		&token.Token, &token.UserLogin, &token.ExpiresAt, &token.Revoked,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, errors.New("refresh token not found")
	}
	if err != nil {
		return nil, fmt.Errorf("querying refresh token: %w", err)
	}

	token.ExpiresAt = token.ExpiresAt.UTC()

	return &token, nil
}

// FindByUserLogin returns refresh tokens for a given user login.
func (r *RefreshTokenRepository) FindByUserLogin(
	ctx context.Context,
	userLogin string,
) ([]*domain.RefreshToken, error) {
	const query = `
		SELECT token, user_login, expires_at, revoked
		FROM refresh_tokens
		WHERE user_login = $1
		ORDER BY created_at DESC
	`

	rows, err := r.db.QueryContext(ctx, query, userLogin)
	if err != nil {
		return nil, fmt.Errorf("querying refresh tokens by user login: %w", err)
	}
	defer func() { _ = rows.Close() }()

	var tokens []*domain.RefreshToken
	for rows.Next() {
		var t domain.RefreshToken
		if err := rows.Scan(&t.Token, &t.UserLogin, &t.ExpiresAt, &t.Revoked); err != nil {
			return nil, fmt.Errorf("scanning refresh token row: %w", err)
		}
		t.ExpiresAt = t.ExpiresAt.UTC()
		tokens = append(tokens, &t)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterating refresh token rows: %w", err)
	}

	return tokens, nil
}

// Delete removes a refresh token.
func (r *RefreshTokenRepository) Delete(
	ctx context.Context,
	token *domain.RefreshToken,
) error {
	res, err := r.db.ExecContext(ctx, `DELETE FROM refresh_tokens WHERE token = $1`, token.Token)
	if err != nil {
		return fmt.Errorf("deleting refresh token: %w", err)
	}

	if n, _ := res.RowsAffected(); n == 0 {
		return errors.New("refresh token not found")
	}

	return nil
}

// DeleteExpiredTokens removes all expired tokens (for a cleanup job).
func (r *RefreshTokenRepository) DeleteExpiredTokens(ctx context.Context) error {
	_, err := r.db.ExecContext(ctx, `DELETE FROM refresh_tokens WHERE expires_at < $1`, time.Now().UTC())
	if err != nil {
		return fmt.Errorf("deleting expired refresh tokens: %w", err)
	}

	return nil
}
