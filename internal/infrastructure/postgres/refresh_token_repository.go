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
	query := `
		INSERT INTO refresh_tokens (
			token,
			user_login,
			expires_at,
			revoked,
			created_at
		)
		VALUES ($1,$2,$3,$4,NOW())
	`

	_, err := r.db.ExecContext(
		ctx,
		query,
		token.Token,
		token.UserLogin,
		token.ExpiresAt,
		token.Revoked,
	)
	if err != nil {
		return nil, fmt.Errorf("saving refresh token: %w", err)
	}

	return token, nil
}

// FindByToken returns a refresh token by token value.
func (r *RefreshTokenRepository) FindByToken(
	ctx context.Context,
	value string,
) (*domain.RefreshToken, error) {
	query := `
		SELECT
			token,
			user_login,
			expires_at,
			revoked
		FROM refresh_tokens
		WHERE token = $1
		LIMIT 1
	`

	row := r.db.QueryRowContext(ctx, query, value)

	var token domain.RefreshToken

	err := row.Scan(
		&token.Token,
		&token.UserLogin,
		&token.ExpiresAt,
		&token.Revoked,
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
	query := `
		SELECT
			token,
			user_login,
			expires_at,
			revoked
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

		if err := rows.Scan(
			&t.Token,
			&t.UserLogin,
			&t.ExpiresAt,
			&t.Revoked,
		); err != nil {
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
	query := `
		DELETE FROM refresh_tokens
		WHERE token = $1
	`

	res, err := r.db.ExecContext(ctx, query, token.Token)
	if err != nil {
		return fmt.Errorf("deleting refresh token: %w", err)
	}

	rows, err := res.RowsAffected()
	if err != nil {
		return fmt.Errorf("checking rows affected: %w", err)
	}

	if rows == 0 {
		return errors.New("refresh token not found")
	}

	return nil
}

// Revoke marks a token as revoked.
func (r *RefreshTokenRepository) Revoke(
	ctx context.Context,
	value string,
) error {
	query := `
		UPDATE refresh_tokens
		SET revoked = TRUE
		WHERE token = $1
	`

	_, err := r.db.ExecContext(ctx, query, value)
	if err != nil {
		return fmt.Errorf("revoking refresh token: %w", err)
	}

	return nil
}

// DeleteExpired removes expired tokens.
func (r *RefreshTokenRepository) DeleteExpiredTokens(
	ctx context.Context,
) error {
	query := `
		DELETE FROM refresh_tokens
		WHERE expires_at < $1
	`

	_, err := r.db.ExecContext(ctx, query, time.Now().UTC())
	if err != nil {
		return fmt.Errorf("deleting expired refresh tokens: %w", err)
	}

	return nil
}
