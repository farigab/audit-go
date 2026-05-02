CREATE TABLE IF NOT EXISTS audit_events (
    id          UUID PRIMARY KEY,
    actor_id    TEXT NOT NULL REFERENCES users(login) ON UPDATE CASCADE,
    action      TEXT NOT NULL CONSTRAINT audit_events_action_check CHECK (
        action IN (
            'jv.created',
            'jv.activated',
            'jv.suspended',
            'document.uploaded',
            'document.deleted',
            'document.parsed',
            'chat.queried'
        )
    ),
    target_id   UUID NOT NULL,
    target_type TEXT NOT NULL CONSTRAINT audit_events_target_type_check CHECK (
        target_type IN ('joint_venture', 'document', 'chat')
    ),
    occurred_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    request_id  UUID NOT NULL,
    metadata    JSONB NOT NULL DEFAULT '{}'
);

CREATE INDEX IF NOT EXISTS idx_audit_events_target_id
    ON audit_events(target_id);

CREATE INDEX IF NOT EXISTS idx_audit_events_occurred_at
    ON audit_events(occurred_at DESC);
