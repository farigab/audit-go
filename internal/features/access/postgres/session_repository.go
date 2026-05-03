package postgres

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/lib/pq"

	"audit-go/internal/features/access"
	accessapp "audit-go/internal/features/access/app"
)

// SessionRepository stores OAuth state, app sessions, and refresh tokens.
type SessionRepository struct {
	db                *sql.DB
	principalCacheTTL time.Duration
	principalCache    map[string]cachedPrincipal
	cacheMu           sync.RWMutex
	now               func() time.Time
}

type cachedPrincipal struct {
	principal access.Principal
	expiresAt time.Time
}

// SessionRepositoryOption customizes a SessionRepository.
type SessionRepositoryOption func(*SessionRepository)

// WithPrincipalCacheTTL enables a short-lived in-memory principal cache.
func WithPrincipalCacheTTL(ttl time.Duration) SessionRepositoryOption {
	return func(r *SessionRepository) {
		if ttl > 0 {
			r.principalCacheTTL = ttl
		}
	}
}

// NewSessionRepository creates a PostgreSQL-backed session repository.
func NewSessionRepository(db *sql.DB, options ...SessionRepositoryOption) *SessionRepository {
	repo := &SessionRepository{
		db:             db,
		principalCache: make(map[string]cachedPrincipal),
		now:            time.Now,
	}
	for _, option := range options {
		option(repo)
	}
	return repo
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
	r.deleteCachedPrincipal(user.Login)
	return nil
}

// PrincipalByLogin loads an application principal and its app roles.
func (r *SessionRepository) PrincipalByLogin(ctx context.Context, login string) (access.Principal, error) {
	if principal, ok := r.cachedPrincipal(login); ok {
		return principal, nil
	}

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
	r.storeCachedPrincipal(login, principal)

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
		INSERT INTO access_sessions (
			token_hash,
			session_id,
			user_login,
			ip_address,
			user_agent,
			last_seen_at,
			expires_at
		)
		VALUES ($1,$2,$3,$4,$5,$6,$7)
	`

	_, err := r.db.ExecContext(
		ctx,
		query,
		record.TokenHash,
		nullableString(record.SessionID),
		record.UserLogin,
		nullableString(record.IPAddress),
		nullableString(record.UserAgent),
		record.LastSeenAt,
		record.ExpiresAt,
	)
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
		UPDATE access_sessions
		SET last_seen_at = CASE
			WHEN last_seen_at IS NULL OR last_seen_at < ($2 - INTERVAL '1 minute') THEN $2
			ELSE last_seen_at
		END
		WHERE token_hash = $1
		  AND revoked_at IS NULL
		  AND expires_at > $2
		RETURNING user_login
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
	const query = `
		UPDATE access_sessions
		SET revoked_at = NOW()
		WHERE token_hash = $1
		  AND revoked_at IS NULL
	`

	_, err := r.db.ExecContext(ctx, query, tokenHash)
	if err != nil {
		return fmt.Errorf("revoking session: %w", err)
	}
	return nil
}

// CreateRefreshToken stores an opaque rotating refresh token.
func (r *SessionRepository) CreateRefreshToken(ctx context.Context, record accessapp.RefreshTokenRecord) error {
	const query = `
		INSERT INTO access_refresh_tokens (token_hash, session_id, user_login, expires_at)
		VALUES ($1,$2,$3,$4)
	`

	_, err := r.db.ExecContext(ctx, query, record.TokenHash, nullableString(record.SessionID), record.UserLogin, record.ExpiresAt)
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
) (accessapp.RefreshTokenIdentity, error) {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return accessapp.RefreshTokenIdentity{}, fmt.Errorf("beginning refresh rotation: %w", err)
	}
	defer func() {
		if err != nil {
			_ = tx.Rollback()
		}
	}()

	const selectQuery = `
		SELECT user_login, session_id, revoked_at, replaced_by_hash, expires_at
		FROM access_refresh_tokens
		WHERE token_hash = $1
		FOR UPDATE
	`

	var identity accessapp.RefreshTokenIdentity
	var sessionID sql.NullString
	var revokedAt sql.NullTime
	var replacedByHash sql.NullString
	var expiresAt time.Time
	err = tx.QueryRowContext(ctx, selectQuery, oldHash).Scan(
		&identity.UserLogin,
		&sessionID,
		&revokedAt,
		&replacedByHash,
		&expiresAt,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return accessapp.RefreshTokenIdentity{}, errors.New("refresh token not found")
	}
	if err != nil {
		return accessapp.RefreshTokenIdentity{}, fmt.Errorf("loading refresh token: %w", err)
	}
	identity.SessionID = sessionID.String

	if !expiresAt.After(now) {
		return accessapp.RefreshTokenIdentity{}, errors.New("refresh token expired")
	}
	if revokedAt.Valid {
		if replacedByHash.Valid {
			if revokeErr := r.revokeSessionFamilyTx(ctx, tx, identity.UserLogin, identity.SessionID); revokeErr != nil {
				return accessapp.RefreshTokenIdentity{}, fmt.Errorf("revoking reused refresh token family: %w", revokeErr)
			}
			err = accessapp.ErrRefreshReuse
			return accessapp.RefreshTokenIdentity{}, err
		}
		return accessapp.RefreshTokenIdentity{}, errors.New("refresh token revoked")
	}
	if identity.SessionID == "" {
		identity.SessionID = next.SessionID
	}

	const updateQuery = `
		UPDATE access_refresh_tokens
		SET revoked_at = NOW(),
			replaced_by_hash = $2
		WHERE token_hash = $1
		  AND revoked_at IS NULL
	`
	if _, err = tx.ExecContext(ctx, updateQuery, oldHash, next.TokenHash); err != nil {
		return accessapp.RefreshTokenIdentity{}, fmt.Errorf("revoking old refresh token: %w", err)
	}

	const insertQuery = `
		INSERT INTO access_refresh_tokens (token_hash, session_id, user_login, expires_at)
		VALUES ($1,$2,$3,$4)
	`
	if _, err = tx.ExecContext(ctx, insertQuery, next.TokenHash, nullableString(identity.SessionID), identity.UserLogin, next.ExpiresAt); err != nil {
		return accessapp.RefreshTokenIdentity{}, fmt.Errorf("creating rotated refresh token: %w", err)
	}

	if err = tx.Commit(); err != nil {
		return accessapp.RefreshTokenIdentity{}, fmt.Errorf("committing refresh rotation: %w", err)
	}

	return identity, nil
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
		  AND revoked_at IS NULL
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
	const query = `
		UPDATE access_refresh_tokens
		SET revoked_at = NOW()
		WHERE token_hash = $1
		  AND revoked_at IS NULL
	`

	_, err := r.db.ExecContext(ctx, query, tokenHash)
	if err != nil {
		return fmt.Errorf("revoking refresh token: %w", err)
	}
	return nil
}

// CleanupExpired deletes expired auth states, sessions, and refresh tokens.
func (r *SessionRepository) CleanupExpired(ctx context.Context, now time.Time) (int64, error) {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return 0, fmt.Errorf("beginning auth cleanup: %w", err)
	}
	defer func() {
		if err != nil {
			_ = tx.Rollback()
		}
	}()

	tables := []string{"access_auth_states", "access_sessions", "access_refresh_tokens"}
	var total int64
	for _, table := range tables {
		result, execErr := tx.ExecContext(ctx, fmt.Sprintf("DELETE FROM %s WHERE expires_at <= $1", table), now)
		if execErr != nil {
			err = fmt.Errorf("cleaning expired rows from %s: %w", table, execErr)
			return 0, err
		}
		affected, rowsErr := result.RowsAffected()
		if rowsErr != nil {
			err = fmt.Errorf("reading cleanup rows from %s: %w", table, rowsErr)
			return 0, err
		}
		total += affected
	}

	if err = tx.Commit(); err != nil {
		return 0, fmt.Errorf("committing auth cleanup: %w", err)
	}

	return total, nil
}

func (r *SessionRepository) revokeSessionFamilyTx(
	ctx context.Context,
	tx *sql.Tx,
	userLogin string,
	sessionID string,
) error {
	var sessionQuery string
	var refreshQuery string
	var args []any
	if sessionID != "" {
		sessionQuery = `
			UPDATE access_sessions
			SET revoked_at = NOW()
			WHERE session_id = $1
			  AND revoked_at IS NULL
		`
		refreshQuery = `
			UPDATE access_refresh_tokens
			SET revoked_at = NOW()
			WHERE session_id = $1
			  AND revoked_at IS NULL
		`
		args = []any{sessionID}
	} else {
		sessionQuery = `
			UPDATE access_sessions
			SET revoked_at = NOW()
			WHERE user_login = $1
			  AND session_id IS NULL
			  AND revoked_at IS NULL
		`
		refreshQuery = `
			UPDATE access_refresh_tokens
			SET revoked_at = NOW()
			WHERE user_login = $1
			  AND session_id IS NULL
			  AND revoked_at IS NULL
		`
		args = []any{userLogin}
	}
	if _, err := tx.ExecContext(ctx, sessionQuery, args...); err != nil {
		return fmt.Errorf("revoking session family sessions: %w", err)
	}
	if _, err := tx.ExecContext(ctx, refreshQuery, args...); err != nil {
		return fmt.Errorf("revoking session family refresh tokens: %w", err)
	}
	return nil
}

func (r *SessionRepository) cachedPrincipal(login string) (access.Principal, bool) {
	if r.principalCacheTTL <= 0 {
		return access.Principal{}, false
	}
	now := r.now()
	r.cacheMu.RLock()
	entry, ok := r.principalCache[login]
	r.cacheMu.RUnlock()
	if !ok || !entry.expiresAt.After(now) {
		if ok {
			r.deleteCachedPrincipal(login)
		}
		return access.Principal{}, false
	}
	return entry.principal, true
}

func (r *SessionRepository) storeCachedPrincipal(login string, principal access.Principal) {
	if r.principalCacheTTL <= 0 {
		return
	}
	r.cacheMu.Lock()
	r.principalCache[login] = cachedPrincipal{
		principal: principal,
		expiresAt: r.now().Add(r.principalCacheTTL),
	}
	r.cacheMu.Unlock()
}

func (r *SessionRepository) deleteCachedPrincipal(login string) {
	r.cacheMu.Lock()
	delete(r.principalCache, login)
	r.cacheMu.Unlock()
}
