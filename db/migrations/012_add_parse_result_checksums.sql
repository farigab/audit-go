ALTER TABLE document_parse_results
    ADD COLUMN IF NOT EXISTS text_sha256 TEXT,
    ADD COLUMN IF NOT EXISTS markdown_sha256 TEXT,
    ADD COLUMN IF NOT EXISTS tables_sha256 TEXT;

DO $$
BEGIN
    IF NOT EXISTS (
        SELECT 1
        FROM pg_constraint
        WHERE conname = 'document_parse_results_text_sha256_check'
    ) THEN
        ALTER TABLE document_parse_results
            ADD CONSTRAINT document_parse_results_text_sha256_check CHECK (
                text_sha256 IS NULL OR length(text_sha256) = 64
            );
    END IF;

    IF NOT EXISTS (
        SELECT 1
        FROM pg_constraint
        WHERE conname = 'document_parse_results_markdown_sha256_check'
    ) THEN
        ALTER TABLE document_parse_results
            ADD CONSTRAINT document_parse_results_markdown_sha256_check CHECK (
                markdown_sha256 IS NULL OR length(markdown_sha256) = 64
            );
    END IF;

    IF NOT EXISTS (
        SELECT 1
        FROM pg_constraint
        WHERE conname = 'document_parse_results_tables_sha256_check'
    ) THEN
        ALTER TABLE document_parse_results
            ADD CONSTRAINT document_parse_results_tables_sha256_check CHECK (
                tables_sha256 IS NULL OR length(tables_sha256) = 64
            );
    END IF;
END $$;
