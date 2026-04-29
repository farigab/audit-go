// Package postgres provides PostgreSQL repository implementations.
package postgres

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"errors"
	"fmt"
	"time"

	"audit-go/internal/domain"
)

type RefreshTokenRepository struct {
	db *sql.DB
}

// hashToken returns the hex-encoded SHA256 of the provided token.
func hashToken(t string) string {
	sum := sha256.Sum256([]byte(t))
	return hex.EncodeToString(sum[:])
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

	// Hash the token before persisting so the DB never contains raw values.
	hashed := hashToken(token.Token)
	_, err := r.db.ExecContext(ctx, query, hashed, token.UserLogin, token.ExpiresAt, token.Revoked)
	if err != nil {
		return nil, fmt.Errorf("saving refresh token: %w", err)
	}

	// Mutate the provided token to hold the stored (hashed) value to keep
	// subsequent operations consistent.
	token.Token = hashed
	return token, nil
}

// Rotate — parâmetro renomeado de `new` para `next` (new é builtin do Go)
func (r *RefreshTokenRepository) Rotate(
	ctx context.Context,
	old, next *domain.RefreshToken,
) error {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("beginning rotation transaction: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	hashedOld := old.Token
	hashedNext := hashToken(next.Token)

	res, err := tx.ExecContext(ctx,
		`UPDATE refresh_tokens SET revoked = TRUE WHERE token = $1`, hashedOld)
	if err != nil {
		return fmt.Errorf("revoking old refresh token: %w", err)
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return errors.New("refresh token not found or already revoked")
	}

	_, err = tx.ExecContext(ctx,
		`INSERT INTO refresh_tokens (token, user_login, expires_at, revoked, created_at)
		 VALUES ($1, $2, $3, $4, NOW())`,
		hashedNext, next.UserLogin, next.ExpiresAt, next.Revoked,
	)
	if err != nil {
		return fmt.Errorf("inserting new refresh token: %w", err)
	}

	next.Token = hashedNext

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

	// Hash the incoming raw token value before querying the DB.
	hashed := hashToken(value)

	var token domain.RefreshToken
	err := r.db.QueryRowContext(ctx, query, hashed).Scan(
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
	// Soft-revoke the token so that reuse can be detected and audited.
	res, err := r.db.ExecContext(ctx, `UPDATE refresh_tokens SET revoked = TRUE WHERE token = $1`, token.Token)
	if err != nil {
		return fmt.Errorf("revoking refresh token: %w", err)
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
