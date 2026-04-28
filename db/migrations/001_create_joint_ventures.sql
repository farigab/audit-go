CREATE TABLE IF NOT EXISTS joint_ventures (
    id          UUID PRIMARY KEY,
    name        TEXT NOT NULL,
    parties     TEXT[]  NOT NULL DEFAULT '{}',
    status      TEXT NOT NULL DEFAULT 'draft',
    created_by  TEXT NOT NULL,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at  TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    metadata    JSONB NOT NULL DEFAULT '{}'
);

CREATE INDEX idx_joint_ventures_status    ON joint_ventures(status);
