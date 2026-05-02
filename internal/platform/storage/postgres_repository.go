package storage

import (
	"context"
	"database/sql"
	"fmt"

	platformpostgres "audit-go/internal/platform/postgres"
)

// Repository persists storage object metadata in PostgreSQL.
type Repository struct {
	db *sql.DB
}

// NewRepository creates a PostgreSQL-backed storage repository.
func NewRepository(db *sql.DB) *Repository {
	return &Repository{db: db}
}

// Save stores object metadata. Container + key are idempotent.
func (r *Repository) Save(ctx context.Context, object Object) error {
	const query = `
		INSERT INTO storage_objects (
			id,
			owner_type,
			owner_id,
			container,
			storage_key,
			filename,
			content_type,
			size_bytes,
			checksum_sha256,
			etag,
			version_id,
			verified_at,
			kind,
			created_by,
			created_at
		)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15)
		ON CONFLICT (container, storage_key) DO UPDATE SET
			owner_type = EXCLUDED.owner_type,
			owner_id = EXCLUDED.owner_id,
			filename = EXCLUDED.filename,
			content_type = EXCLUDED.content_type,
			size_bytes = EXCLUDED.size_bytes,
			checksum_sha256 = EXCLUDED.checksum_sha256,
			etag = EXCLUDED.etag,
			version_id = EXCLUDED.version_id,
			verified_at = EXCLUDED.verified_at,
			kind = EXCLUDED.kind
	`

	_, err := platformpostgres.Executor(ctx, r.db).ExecContext(
		ctx,
		query,
		object.ID,
		string(object.OwnerType),
		object.OwnerID,
		object.Container,
		object.StorageKey,
		object.Filename,
		nullableString(object.ContentType),
		object.SizeBytes,
		nullableString(object.ChecksumSHA256),
		nullableString(object.ETag),
		nullableString(object.VersionID),
		object.VerifiedAt,
		string(object.Kind),
		object.CreatedBy,
		object.CreatedAt,
	)
	if err != nil {
		return fmt.Errorf("saving storage object: %w", err)
	}

	return nil
}

func nullableString(value string) any {
	if value == "" {
		return nil
	}
	return value
}
