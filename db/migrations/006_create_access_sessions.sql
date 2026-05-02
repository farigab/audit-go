CREATE TABLE IF NOT EXISTS access_auth_states (
    state_hash    TEXT PRIMARY KEY,
    code_verifier TEXT NOT NULL,
    nonce         TEXT NOT NULL,
    return_url    TEXT NOT NULL,
    expires_at    TIMESTAMPTZ NOT NULL,
    created_at    TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_access_auth_states_expires_at
    ON access_auth_states(expires_at);

CREATE TABLE IF NOT EXISTS access_sessions (
    token_hash TEXT PRIMARY KEY,
    user_login TEXT NOT NULL REFERENCES users(login) ON DELETE CASCADE,
    expires_at TIMESTAMPTZ NOT NULL,
    revoked    BOOLEAN NOT NULL DEFAULT FALSE,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_access_sessions_user_login
    ON access_sessions(user_login);

CREATE INDEX IF NOT EXISTS idx_access_sessions_expires_at
    ON access_sessions(expires_at);

CREATE TABLE IF NOT EXISTS access_refresh_tokens (
    token_hash       TEXT PRIMARY KEY,
    user_login       TEXT NOT NULL REFERENCES users(login) ON DELETE CASCADE,
    expires_at       TIMESTAMPTZ NOT NULL,
    revoked          BOOLEAN NOT NULL DEFAULT FALSE,
    replaced_by_hash TEXT,
    created_at       TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_access_refresh_tokens_user_login
    ON access_refresh_tokens(user_login);

CREATE INDEX IF NOT EXISTS idx_access_refresh_tokens_expires_at
    ON access_refresh_tokens(expires_at);
