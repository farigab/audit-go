CREATE TABLE IF NOT EXISTS prompts (
    id                UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    name              TEXT NOT NULL,
    description       TEXT NOT NULL DEFAULT '',
    category          TEXT NOT NULL DEFAULT 'audit_chat',
    active_version_id UUID,
    created_by        TEXT NOT NULL REFERENCES users(login) ON UPDATE CASCADE,
    created_at        TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at        TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TABLE IF NOT EXISTS prompt_versions (
    id            UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    prompt_id     UUID NOT NULL REFERENCES prompts(id) ON DELETE CASCADE,
    version       INTEGER NOT NULL,
    system_prompt TEXT NOT NULL,
    user_template TEXT NOT NULL,
    model         TEXT NOT NULL DEFAULT 'gpt-4o-mini',
    temperature   NUMERIC(3,2) NOT NULL DEFAULT 0.20 CONSTRAINT prompt_versions_temperature_check CHECK (
        temperature >= 0 AND temperature <= 2
    ),
    status        TEXT NOT NULL DEFAULT 'draft' CONSTRAINT prompt_versions_status_check CHECK (
        status IN ('draft', 'approved', 'deprecated')
    ),
    created_by    TEXT NOT NULL REFERENCES users(login) ON UPDATE CASCADE,
    created_at    TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    approved_by   TEXT REFERENCES users(login) ON UPDATE CASCADE,
    approved_at   TIMESTAMPTZ,
    deprecated_at TIMESTAMPTZ,
    UNIQUE (prompt_id, version)
);

DO $$
BEGIN
    IF NOT EXISTS (
        SELECT 1
        FROM pg_constraint
        WHERE conname = 'prompts_active_version_id_fkey'
    ) THEN
        ALTER TABLE prompts
            ADD CONSTRAINT prompts_active_version_id_fkey
            FOREIGN KEY (active_version_id) REFERENCES prompt_versions(id) ON DELETE SET NULL;
    END IF;
END $$;

CREATE TABLE IF NOT EXISTS prompt_runs (
    id                UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    prompt_version_id UUID REFERENCES prompt_versions(id) ON DELETE SET NULL,
    jv_id             UUID NOT NULL REFERENCES joint_ventures(id) ON DELETE CASCADE,
    question          TEXT NOT NULL,
    answer            TEXT NOT NULL,
    context_bytes     INTEGER NOT NULL DEFAULT 0,
    created_by        TEXT NOT NULL REFERENCES users(login) ON UPDATE CASCADE,
    created_at        TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_prompt_versions_prompt_id
    ON prompt_versions(prompt_id, version DESC);

CREATE INDEX IF NOT EXISTS idx_prompt_runs_jv_id_created_at
    ON prompt_runs(jv_id, created_at DESC);
