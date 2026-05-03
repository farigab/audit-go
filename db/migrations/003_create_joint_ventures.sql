CREATE TABLE IF NOT EXISTS joint_ventures (
    id          UUID PRIMARY KEY,
    region_id   UUID NOT NULL REFERENCES regions(id),
    name        TEXT NOT NULL,
    parties     TEXT[] NOT NULL DEFAULT '{}',
    status      TEXT NOT NULL DEFAULT 'draft' CONSTRAINT joint_ventures_status_check CHECK (
        status IN ('draft', 'active', 'suspended', 'closed')
    ),
    created_by  TEXT NOT NULL REFERENCES users(login) ON UPDATE CASCADE,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at  TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    metadata    JSONB NOT NULL DEFAULT '{}'
);

CREATE INDEX IF NOT EXISTS idx_joint_ventures_region_id
    ON joint_ventures(region_id);

CREATE INDEX IF NOT EXISTS idx_joint_ventures_status
    ON joint_ventures(status);
