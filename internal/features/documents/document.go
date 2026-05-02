// Package documents owns document metadata and lifecycle concepts.
package documents

import "time"

// Status describes the document processing lifecycle.
type Status string

const (
	StatusUploadPending Status = "upload_pending"
	StatusUploaded      Status = "uploaded"
	StatusRegistered    Status = "registered"
	StatusQueued        Status = "queued"
	StatusProcessing    Status = "processing"
	StatusParsed        Status = "parsed"
	StatusOCRCompleted  Status = "ocr_completed"
	StatusIndexed       Status = "indexed"
	StatusFailed        Status = "failed"
	StatusDeleted       Status = "deleted"
)

// Type identifies the document type.
type Type string

const (
	TypeContract  Type = "contract"
	TypeFinancial Type = "financial"
	TypeReport    Type = "report"
	TypeOther     Type = "other"
)

// Document represents a persisted document entity.
type Document struct {
	ID         string    `json:"id"`
	JVID       string    `json:"jv_id"`
	Name       string    `json:"name"`
	Type       Type      `json:"type"`
	StorageKey string    `json:"storage_key"`
	UploadedBy string    `json:"uploaded_by"`
	UploadedAt time.Time `json:"uploaded_at"`
	Status     Status    `json:"status"`
	Processed  bool      `json:"processed"`
}

// IsValidType reports whether t is a supported document type.
func IsValidType(t Type) bool {
	switch t {
	case TypeContract, TypeFinancial, TypeReport, TypeOther:
		return true
	default:
		return false
	}
}

// NormalizeType returns TypeOther when no type was supplied.
func NormalizeType(t Type) Type {
	if t == "" {
		return TypeOther
	}
	return t
}

// NormalizeStatus returns StatusRegistered when no status was supplied.
func NormalizeStatus(s Status) Status {
	if s == "" {
		return StatusRegistered
	}
	return s
}

// IsProcessed reports whether the document has been processed.
func (d Document) IsProcessed() bool {
	return d.Processed
}
