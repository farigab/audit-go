CREATE TABLE IF NOT EXISTS documents (
    id          UUID PRIMARY KEY,
    jv_id       UUID NOT NULL REFERENCES joint_ventures(id) ON DELETE CASCADE,
    name        TEXT NOT NULL,
    type        TEXT NOT NULL DEFAULT 'other' CONSTRAINT documents_type_check CHECK (
        type IN ('contract', 'financial', 'report', 'other')
    ),
    storage_key TEXT NOT NULL,
    uploaded_by TEXT NOT NULL REFERENCES users(login) ON UPDATE CASCADE,
    uploaded_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    processed   BOOLEAN NOT NULL DEFAULT FALSE
);

CREATE INDEX IF NOT EXISTS idx_documents_jv_id
    ON documents(jv_id);

CREATE INDEX IF NOT EXISTS idx_documents_processed
    ON documents(processed)
    WHERE processed = FALSE;
