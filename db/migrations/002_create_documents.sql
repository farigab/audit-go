CREATE TABLE IF NOT EXISTS documents (
    id          UUID PRIMARY KEY,
    jv_id       UUID NOT NULL REFERENCES joint_ventures(id) ON DELETE CASCADE,
    tenant_id   UUID NOT NULL,
    name        TEXT NOT NULL,
    type        TEXT NOT NULL DEFAULT 'other',
    storage_key TEXT NOT NULL,
    uploaded_by TEXT NOT NULL,
    uploaded_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    processed   BOOLEAN NOT NULL DEFAULT FALSE
);

CREATE INDEX idx_documents_jv_id     ON documents(jv_id);
CREATE INDEX idx_documents_tenant_id ON documents(tenant_id);
CREATE INDEX idx_documents_processed ON documents(processed) WHERE processed = FALSE;
