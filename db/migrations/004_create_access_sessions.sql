CREATE TABLE IF NOT EXISTS access_auth_states (
    state_hash    TEXT PRIMARY KEY CONSTRAINT access_auth_states_state_hash_check CHECK (length(state_hash) = 64),
    code_verifier TEXT NOT NULL,
    nonce         TEXT NOT NULL,
    return_url    TEXT NOT NULL,
    expires_at    TIMESTAMPTZ NOT NULL,
    created_at    TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_access_auth_states_expires_at
    ON access_auth_states(expires_at);

CREATE TABLE IF NOT EXISTS access_sessions (
    token_hash   TEXT PRIMARY KEY CONSTRAINT access_sessions_token_hash_check CHECK (length(token_hash) = 64),
    session_id   TEXT,
    user_login   TEXT NOT NULL REFERENCES users(login) ON UPDATE CASCADE ON DELETE CASCADE,
    ip_address   TEXT,
    user_agent   TEXT,
    last_seen_at TIMESTAMPTZ,
    expires_at   TIMESTAMPTZ NOT NULL,
    revoked_at   TIMESTAMPTZ,
    created_at   TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_access_sessions_user_login
    ON access_sessions(user_login);

CREATE INDEX IF NOT EXISTS idx_access_sessions_session_id
    ON access_sessions(session_id);

CREATE INDEX IF NOT EXISTS idx_access_sessions_last_seen_at
    ON access_sessions(last_seen_at);

CREATE INDEX IF NOT EXISTS idx_access_sessions_revoked_at
    ON access_sessions(revoked_at);

CREATE INDEX IF NOT EXISTS idx_access_sessions_expires_at
    ON access_sessions(expires_at);

CREATE TABLE IF NOT EXISTS access_refresh_tokens (
    token_hash       TEXT PRIMARY KEY CONSTRAINT access_refresh_tokens_token_hash_check CHECK (length(token_hash) = 64),
    session_id       TEXT,
    user_login       TEXT NOT NULL REFERENCES users(login) ON UPDATE CASCADE ON DELETE CASCADE,
    expires_at       TIMESTAMPTZ NOT NULL,
    revoked_at       TIMESTAMPTZ,
    replaced_by_hash TEXT CONSTRAINT access_refresh_tokens_replaced_by_hash_check CHECK (
        replaced_by_hash IS NULL OR length(replaced_by_hash) = 64
    ),
    created_at       TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_access_refresh_tokens_user_login
    ON access_refresh_tokens(user_login);

CREATE INDEX IF NOT EXISTS idx_access_refresh_tokens_session_id
    ON access_refresh_tokens(session_id);

CREATE INDEX IF NOT EXISTS idx_access_refresh_tokens_revoked_at
    ON access_refresh_tokens(revoked_at);

CREATE INDEX IF NOT EXISTS idx_access_refresh_tokens_expires_at
    ON access_refresh_tokens(expires_at);
