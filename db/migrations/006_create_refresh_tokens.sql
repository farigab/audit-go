CREATE TABLE IF NOT EXISTS refresh_tokens (
    token      TEXT PRIMARY KEY,
    user_login TEXT NOT NULL REFERENCES users(login) ON DELETE CASCADE,
    expires_at TIMESTAMPTZ NOT NULL,
    revoked    BOOLEAN NOT NULL DEFAULT FALSE,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_refresh_tokens_user_login ON refresh_tokens(user_login);
CREATE INDEX idx_refresh_tokens_expires_at ON refresh_tokens(expires_at);
