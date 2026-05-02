ALTER TABLE documents
    ADD COLUMN IF NOT EXISTS status TEXT NOT NULL DEFAULT 'registered';

DO $$
BEGIN
    IF NOT EXISTS (
        SELECT 1
        FROM pg_constraint
        WHERE conname = 'documents_status_check'
    ) THEN
        ALTER TABLE documents
            ADD CONSTRAINT documents_status_check CHECK (
                status IN (
                    'upload_pending',
                    'uploaded',
                    'registered',
                    'queued',
                    'processing',
                    'parsed',
                    'ocr_completed',
                    'indexed',
                    'failed',
                    'deleted'
                )
            );
    END IF;
END $$;

CREATE INDEX IF NOT EXISTS idx_documents_status
    ON documents(status);

CREATE TABLE IF NOT EXISTS storage_objects (
    id              UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    owner_type      TEXT NOT NULL CONSTRAINT storage_objects_owner_type_check CHECK (
        owner_type IN ('document', 'audit_run', 'report')
    ),
    owner_id        UUID NOT NULL,
    container       TEXT NOT NULL,
    storage_key     TEXT NOT NULL,
    filename        TEXT NOT NULL,
    content_type    TEXT,
    size_bytes      BIGINT CONSTRAINT storage_objects_size_bytes_check CHECK (
        size_bytes IS NULL OR size_bytes >= 0
    ),
    checksum_sha256 TEXT CONSTRAINT storage_objects_checksum_sha256_check CHECK (
        checksum_sha256 IS NULL OR length(checksum_sha256) = 64
    ),
    kind            TEXT NOT NULL CONSTRAINT storage_objects_kind_check CHECK (
        kind IN ('raw', 'parsed_text', 'parsed_table', 'report', 'temp')
    ),
    created_by      TEXT NOT NULL REFERENCES users(login) ON UPDATE CASCADE,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT storage_objects_container_key_unique UNIQUE (container, storage_key)
);

CREATE INDEX IF NOT EXISTS idx_storage_objects_owner
    ON storage_objects(owner_type, owner_id);

CREATE TABLE IF NOT EXISTS outbox_events (
    id             UUID PRIMARY KEY,
    event_type     TEXT NOT NULL,
    aggregate_type TEXT NOT NULL,
    aggregate_id   UUID NOT NULL,
    payload        JSONB NOT NULL DEFAULT '{}',
    status         TEXT NOT NULL DEFAULT 'pending' CONSTRAINT outbox_events_status_check CHECK (
        status IN ('pending', 'published', 'failed')
    ),
    attempts       INTEGER NOT NULL DEFAULT 0 CONSTRAINT outbox_events_attempts_check CHECK (
        attempts >= 0
    ),
    last_error     TEXT,
    created_at     TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    published_at   TIMESTAMPTZ
);

CREATE INDEX IF NOT EXISTS idx_outbox_events_pending
    ON outbox_events(created_at)
    WHERE status = 'pending';

CREATE INDEX IF NOT EXISTS idx_outbox_events_aggregate
    ON outbox_events(aggregate_type, aggregate_id);

CREATE TABLE IF NOT EXISTS processing_jobs (
    id              UUID PRIMARY KEY,
    job_type        TEXT NOT NULL,
    aggregate_type  TEXT NOT NULL,
    aggregate_id    UUID NOT NULL,
    status          TEXT NOT NULL DEFAULT 'queued' CONSTRAINT processing_jobs_status_check CHECK (
        status IN ('queued', 'running', 'retry_scheduled', 'completed', 'failed', 'dead_letter')
    ),
    payload         JSONB NOT NULL DEFAULT '{}',
    idempotency_key TEXT NOT NULL UNIQUE,
    attempts        INTEGER NOT NULL DEFAULT 0 CONSTRAINT processing_jobs_attempts_check CHECK (
        attempts >= 0
    ),
    max_attempts    INTEGER NOT NULL DEFAULT 5 CONSTRAINT processing_jobs_max_attempts_check CHECK (
        max_attempts > 0
    ),
    available_at    TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    locked_by       TEXT,
    locked_until    TIMESTAMPTZ,
    last_error      TEXT,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at      TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_processing_jobs_available
    ON processing_jobs(status, available_at)
    WHERE status IN ('queued', 'retry_scheduled');

CREATE INDEX IF NOT EXISTS idx_processing_jobs_aggregate
    ON processing_jobs(aggregate_type, aggregate_id);
