CREATE TABLE IF NOT EXISTS sampling_rule_sets (
    id          UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    jv_id       UUID NOT NULL REFERENCES joint_ventures(id) ON DELETE CASCADE,
    name        TEXT NOT NULL,
    description TEXT NOT NULL DEFAULT '',
    parameters  JSONB NOT NULL DEFAULT '{}'::jsonb,
    qualitative_rules JSONB NOT NULL DEFAULT '[]'::jsonb,
    active      BOOLEAN NOT NULL DEFAULT TRUE,
    created_by  TEXT NOT NULL REFERENCES users(login) ON UPDATE CASCADE,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at  TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_sampling_rule_sets_jv_active
    ON sampling_rule_sets(jv_id, active, created_at DESC);

CREATE TABLE IF NOT EXISTS sampling_runs (
    id          UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    jv_id       UUID NOT NULL REFERENCES joint_ventures(id) ON DELETE CASCADE,
    rule_set_id UUID REFERENCES sampling_rule_sets(id) ON DELETE SET NULL,
    status      TEXT NOT NULL DEFAULT 'completed' CONSTRAINT sampling_runs_status_check CHECK (
        status IN ('completed', 'failed')
    ),
    total_candidates INTEGER NOT NULL DEFAULT 0,
    selected_count   INTEGER NOT NULL DEFAULT 0,
    created_by  TEXT NOT NULL REFERENCES users(login) ON UPDATE CASCADE,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_sampling_runs_jv_created_at
    ON sampling_runs(jv_id, created_at DESC);

CREATE TABLE IF NOT EXISTS sampling_run_items (
    run_id      UUID NOT NULL REFERENCES sampling_runs(id) ON DELETE CASCADE,
    document_id UUID NOT NULL REFERENCES documents(id) ON DELETE CASCADE,
    score       INTEGER NOT NULL DEFAULT 0,
    reasons     JSONB NOT NULL DEFAULT '[]'::jsonb,
    selected_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    PRIMARY KEY (run_id, document_id)
);

ALTER TABLE audit_events
    DROP CONSTRAINT IF EXISTS audit_events_action_check;

ALTER TABLE audit_events
    ADD CONSTRAINT audit_events_action_check CHECK (
        action IN (
            'jv.created',
            'jv.activated',
            'jv.suspended',
            'document.upload_requested',
            'document.uploaded',
            'document.deleted',
            'document.parsed',
            'chat.queried',
            'sampling.rule_set_created',
            'sampling.run_created'
        )
    );

ALTER TABLE audit_events
    DROP CONSTRAINT IF EXISTS audit_events_target_type_check;

ALTER TABLE audit_events
    ADD CONSTRAINT audit_events_target_type_check CHECK (
        target_type IN ('joint_venture', 'document', 'chat', 'sampling')
    );
