ALTER TABLE storage_objects
    ADD COLUMN IF NOT EXISTS etag TEXT,
    ADD COLUMN IF NOT EXISTS version_id TEXT,
    ADD COLUMN IF NOT EXISTS verified_at TIMESTAMPTZ;

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
            'chat.queried'
        )
    );
