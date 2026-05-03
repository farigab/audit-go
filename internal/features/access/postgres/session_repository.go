package postgres

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/lib/pq"

	"audit-go/internal/features/access"
	accessapp "audit-go/internal/features/access/app"
)

// SessionRepository stores OAuth state, app sessions, and refresh tokens.
type SessionRepository struct {
	db *sql.DB
}

// NewSessionRepository creates a PostgreSQL-backed session repository.
func NewSessionRepository(db *sql.DB) *SessionRepository {
	return &SessionRepository{db: db}
}

// CreateAuthState stores one OAuth authorization attempt.
func (r *SessionRepository) CreateAuthState(ctx context.Context, state accessapp.AuthState) error {
	const query = `
		INSERT INTO access_auth_states (
			state_hash,
			code_verifier,
			nonce,
			return_url,
			expires_at
		)
		VALUES ($1,$2,$3,$4,$5)
	`

	_, err := r.db.ExecContext(ctx, query, state.StateHash, state.CodeVerifier, state.Nonce, state.ReturnURL, state.ExpiresAt)
	if err != nil {
		return fmt.Errorf("creating auth state: %w", err)
	}
	return nil
}

// ConsumeAuthState returns and deletes a valid OAuth state.
func (r *SessionRepository) ConsumeAuthState(
	ctx context.Context,
	stateHash string,
	now time.Time,
) (*accessapp.AuthState, error) {
	const query = `
		DELETE FROM access_auth_states
		WHERE state_hash = $1
		  AND expires_at > $2
		RETURNING state_hash, code_verifier, nonce, return_url, expires_at
	`

	var state accessapp.AuthState
	err := r.db.QueryRowContext(ctx, query, stateHash, now).Scan(
		&state.StateHash,
		&state.CodeVerifier,
		&state.Nonce,
		&state.ReturnURL,
		&state.ExpiresAt,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, errors.New("auth state not found or expired")
	}
	if err != nil {
		return nil, fmt.Errorf("consuming auth state: %w", err)
	}
	return &state, nil
}

// UpsertUser creates or updates the application user projected from Entra.
func (r *SessionRepository) UpsertUser(ctx context.Context, user accessapp.UserProfile) error {
	const query = `
		INSERT INTO users (login, entra_oid, name)
		VALUES ($1, $2, $3)
		ON CONFLICT (login) DO UPDATE SET
			entra_oid = EXCLUDED.entra_oid,
			name = EXCLUDED.name,
			updated_at = NOW()
	`

	_, err := r.db.ExecContext(ctx, query, user.Login, nullableString(user.EntraOID), user.Name)
	if err != nil {
		return fmt.Errorf("upserting user: %w", err)
	}
	return nil
}

// PrincipalByLogin loads an application principal and its app roles.
func (r *SessionRepository) PrincipalByLogin(ctx context.Context, login string) (access.Principal, error) {
	const query = `
		SELECT
			u.login,
			COALESCE(u.entra_oid, ''),
			u.name,
			COALESCE(array_remove(array_agg(DISTINCT m.role), NULL), ARRAY[]::text[])
		FROM users u
		LEFT JOIN access_memberships m ON m.user_login = u.login
		WHERE u.login = $1
		GROUP BY u.login, u.entra_oid, u.name
	`

	var principal access.Principal
	var entraOID string
	var roles pq.StringArray
	err := r.db.QueryRowContext(ctx, query, login).Scan(&principal.Login, &entraOID, &principal.Name, &roles)
	if errors.Is(err, sql.ErrNoRows) {
		return access.Principal{}, errors.New("user not found")
	}
	if err != nil {
		return access.Principal{}, fmt.Errorf("loading principal: %w", err)
	}
	principal.ID = entraOID
	if principal.ID == "" {
		principal.ID = principal.Login
	}
	principal.Roles = access.RolesFromStrings([]string(roles))

	return principal, nil
}

func nullableString(value string) any {
	if value == "" {
		return nil
	}
	return value
}

// CreateSession stores an opaque application session.
func (r *SessionRepository) CreateSession(ctx context.Context, record accessapp.SessionRecord) error {
	const query = `
		INSERT INTO access_sessions (token_hash, user_login, expires_at)
		VALUES ($1,$2,$3)
	`

	_, err := r.db.ExecContext(ctx, query, record.TokenHash, record.UserLogin, record.ExpiresAt)
	if err != nil {
		return fmt.Errorf("creating session: %w", err)
	}
	return nil
}

// PrincipalBySession resolves a valid session into an application principal.
func (r *SessionRepository) PrincipalBySession(
	ctx context.Context,
	tokenHash string,
	now time.Time,
) (access.Principal, error) {
	const query = `
		SELECT s.user_login
		FROM access_sessions s
		WHERE s.token_hash = $1
		  AND s.revoked = FALSE
		  AND s.expires_at > $2
	`

	var login string
	if err := r.db.QueryRowContext(ctx, query, tokenHash, now).Scan(&login); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return access.Principal{}, access.ErrUnauthenticated
		}
		return access.Principal{}, fmt.Errorf("loading session: %w", err)
	}

	return r.PrincipalByLogin(ctx, login)
}

// RevokeSession revokes a session token.
func (r *SessionRepository) RevokeSession(ctx context.Context, tokenHash string) error {
	const query = `UPDATE access_sessions SET revoked = TRUE WHERE token_hash = $1`

	_, err := r.db.ExecContext(ctx, query, tokenHash)
	if err != nil {
		return fmt.Errorf("revoking session: %w", err)
	}
	return nil
}

// CreateRefreshToken stores an opaque rotating refresh token.
func (r *SessionRepository) CreateRefreshToken(ctx context.Context, record accessapp.RefreshTokenRecord) error {
	const query = `
		INSERT INTO access_refresh_tokens (token_hash, user_login, expires_at)
		VALUES ($1,$2,$3)
	`

	_, err := r.db.ExecContext(ctx, query, record.TokenHash, record.UserLogin, record.ExpiresAt)
	if err != nil {
		return fmt.Errorf("creating refresh token: %w", err)
	}
	return nil
}

// RotateRefreshToken revokes oldHash and stores next in one transaction.
func (r *SessionRepository) RotateRefreshToken(
	ctx context.Context,
	oldHash string,
	next accessapp.RefreshTokenRecord,
	now time.Time,
) error {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("beginning refresh rotation: %w", err)
	}
	defer func() {
		if err != nil {
			_ = tx.Rollback()
		}
	}()

	const selectQuery = `
		SELECT user_login
		FROM access_refresh_tokens
		WHERE token_hash = $1
		  AND revoked = FALSE
		  AND expires_at > $2
		FOR UPDATE
	`

	var userLogin string
	err = tx.QueryRowContext(ctx, selectQuery, oldHash, now).Scan(&userLogin)
	if errors.Is(err, sql.ErrNoRows) {
		return errors.New("refresh token not found or expired")
	}
	if err != nil {
		return fmt.Errorf("loading refresh token: %w", err)
	}

	const updateQuery = `
		UPDATE access_refresh_tokens
		SET revoked = TRUE,
			replaced_by_hash = $2
		WHERE token_hash = $1
	`
	if _, err = tx.ExecContext(ctx, updateQuery, oldHash, next.TokenHash); err != nil {
		return fmt.Errorf("revoking old refresh token: %w", err)
	}

	const insertQuery = `
		INSERT INTO access_refresh_tokens (token_hash, user_login, expires_at)
		VALUES ($1,$2,$3)
	`
	if _, err = tx.ExecContext(ctx, insertQuery, next.TokenHash, userLogin, next.ExpiresAt); err != nil {
		return fmt.Errorf("creating rotated refresh token: %w", err)
	}

	if err = tx.Commit(); err != nil {
		return fmt.Errorf("committing refresh rotation: %w", err)
	}

	return nil
}

// PrincipalByRefreshToken resolves a valid refresh token into a principal.
func (r *SessionRepository) PrincipalByRefreshToken(
	ctx context.Context,
	tokenHash string,
	now time.Time,
) (access.Principal, error) {
	const query = `
		SELECT user_login
		FROM access_refresh_tokens
		WHERE token_hash = $1
		  AND revoked = FALSE
		  AND expires_at > $2
	`

	var login string
	if err := r.db.QueryRowContext(ctx, query, tokenHash, now).Scan(&login); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return access.Principal{}, access.ErrUnauthenticated
		}
		return access.Principal{}, fmt.Errorf("loading refresh token: %w", err)
	}

	return r.PrincipalByLogin(ctx, login)
}

// RevokeRefreshToken revokes a refresh token.
func (r *SessionRepository) RevokeRefreshToken(ctx context.Context, tokenHash string) error {
	const query = `UPDATE access_refresh_tokens SET revoked = TRUE WHERE token_hash = $1`

	_, err := r.db.ExecContext(ctx, query, tokenHash)
	if err != nil {
		return fmt.Errorf("revoking refresh token: %w", err)
	}
	return nil
}
