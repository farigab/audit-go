CREATE TABLE IF NOT EXISTS users (
    login      TEXT PRIMARY KEY,
    entra_oid  TEXT,
    name       TEXT NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE UNIQUE INDEX IF NOT EXISTS idx_users_entra_oid
    ON users(entra_oid)
    WHERE entra_oid IS NOT NULL;
