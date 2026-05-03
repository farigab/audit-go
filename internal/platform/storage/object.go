// Package storage owns metadata for externally stored objects.
package storage

import "time"

// OwnerType identifies the aggregate that owns a stored object.
type OwnerType string

const (
	OwnerDocument OwnerType = "document"
	OwnerAuditRun OwnerType = "audit_run"
	OwnerReport   OwnerType = "report"
)

// Kind identifies the role of the object in the storage lifecycle.
type Kind string

const (
	KindRaw         Kind = "raw"
	KindParsedText  Kind = "parsed_text"
	KindParsedTable Kind = "parsed_table"
	KindReport      Kind = "report"
	KindTemp        Kind = "temp"
)

// Object is the database metadata for a blob stored outside PostgreSQL.
type Object struct {
	ID             string
	OwnerType      OwnerType
	OwnerID        string
	Container      string
	StorageKey     string
	Filename       string
	ContentType    string
	SizeBytes      *int64
	ChecksumSHA256 string
	ETag           string
	VersionID      string
	VerifiedAt     *time.Time
	Kind           Kind
	CreatedBy      string
	CreatedAt      time.Time
}

// DownloadedBlob contains raw blob bytes plus the metadata returned by storage.
type DownloadedBlob struct {
	Content    []byte
	Properties BlobProperties
}
