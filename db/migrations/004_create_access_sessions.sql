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
    token_hash TEXT PRIMARY KEY CONSTRAINT access_sessions_token_hash_check CHECK (length(token_hash) = 64),
    user_login TEXT NOT NULL REFERENCES users(login) ON UPDATE CASCADE ON DELETE CASCADE,
    expires_at TIMESTAMPTZ NOT NULL,
    revoked    BOOLEAN NOT NULL DEFAULT FALSE,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_access_sessions_user_login
    ON access_sessions(user_login);

CREATE INDEX IF NOT EXISTS idx_access_sessions_expires_at
    ON access_sessions(expires_at);

CREATE TABLE IF NOT EXISTS access_refresh_tokens (
    token_hash       TEXT PRIMARY KEY CONSTRAINT access_refresh_tokens_token_hash_check CHECK (length(token_hash) = 64),
    user_login       TEXT NOT NULL REFERENCES users(login) ON UPDATE CASCADE ON DELETE CASCADE,
    expires_at       TIMESTAMPTZ NOT NULL,
    revoked          BOOLEAN NOT NULL DEFAULT FALSE,
    replaced_by_hash TEXT CONSTRAINT access_refresh_tokens_replaced_by_hash_check CHECK (
        replaced_by_hash IS NULL OR length(replaced_by_hash) = 64
    ),
    created_at       TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_access_refresh_tokens_user_login
    ON access_refresh_tokens(user_login);

CREATE INDEX IF NOT EXISTS idx_access_refresh_tokens_expires_at
    ON access_refresh_tokens(expires_at);
