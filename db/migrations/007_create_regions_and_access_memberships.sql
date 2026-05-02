CREATE EXTENSION IF NOT EXISTS pgcrypto;

CREATE TABLE IF NOT EXISTS regions (
    id          UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    name        TEXT NOT NULL,
    code        TEXT NOT NULL UNIQUE,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

ALTER TABLE joint_ventures
    ADD COLUMN IF NOT EXISTS region_id UUID REFERENCES regions(id);

CREATE INDEX IF NOT EXISTS idx_joint_ventures_region_id
    ON joint_ventures(region_id);

CREATE TABLE IF NOT EXISTS access_memberships (
    id          UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    user_login  TEXT NOT NULL REFERENCES users(login) ON DELETE CASCADE,
    role        TEXT NOT NULL CHECK (
        role IN ('admin', 'region_admin', 'jv_admin', 'contributor', 'auditor', 'visitor')
    ),
    scope_type  TEXT NOT NULL CHECK (
        scope_type IN ('system', 'region', 'joint_venture')
    ),
    scope_id    UUID,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CHECK (
        (scope_type = 'system' AND scope_id IS NULL)
        OR (scope_type IN ('region', 'joint_venture') AND scope_id IS NOT NULL)
    )
);

CREATE UNIQUE INDEX IF NOT EXISTS idx_access_memberships_unique_scope
    ON access_memberships(user_login, role, scope_type, COALESCE(scope_id, '00000000-0000-0000-0000-000000000000'::uuid));

CREATE INDEX IF NOT EXISTS idx_access_memberships_user_login
    ON access_memberships(user_login);

CREATE INDEX IF NOT EXISTS idx_access_memberships_scope
    ON access_memberships(scope_type, scope_id);
