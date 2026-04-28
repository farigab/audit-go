CREATE TABLE IF NOT EXISTS users (
    login         TEXT PRIMARY KEY,
    name          TEXT NOT NULL,
    created_at    TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
