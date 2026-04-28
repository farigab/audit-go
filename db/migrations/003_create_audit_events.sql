CREATE TABLE IF NOT EXISTS audit_events (
    id          UUID PRIMARY KEY,
    tenant_id   UUID NOT NULL,
    actor_id    TEXT NOT NULL,
    action      TEXT NOT NULL,
    target_id   UUID NOT NULL,
    target_type TEXT NOT NULL,
    occurred_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    request_id  UUID NOT NULL,
    metadata    JSONB NOT NULL DEFAULT '{}'
);


CREATE INDEX idx_audit_events_tenant_id  ON audit_events(tenant_id);
CREATE INDEX idx_audit_events_target_id  ON audit_events(target_id);
CREATE INDEX idx_audit_events_occurred_at ON audit_events(occurred_at DESC);
