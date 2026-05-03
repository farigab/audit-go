CREATE TABLE IF NOT EXISTS document_parse_results (
    document_id UUID PRIMARY KEY REFERENCES documents(id) ON DELETE CASCADE,
    filename    TEXT NOT NULL DEFAULT '',
    pages       INTEGER CONSTRAINT document_parse_results_pages_check CHECK (
        pages IS NULL OR pages >= 0
    ),
    text        TEXT NOT NULL DEFAULT '',
    markdown    TEXT NOT NULL DEFAULT '',
    tables      JSONB NOT NULL DEFAULT '[]',
    created_at  TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at  TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_document_parse_results_updated_at
    ON document_parse_results(updated_at DESC);

CREATE INDEX IF NOT EXISTS idx_processing_jobs_running_locks
    ON processing_jobs(locked_until)
    WHERE status = 'running';
