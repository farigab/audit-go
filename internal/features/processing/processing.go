// Package processing owns asynchronous job and outbox concepts.
package processing

import (
	"encoding/json"
	"fmt"
	"time"

	"github.com/google/uuid"
)

const (
	EventDocumentUploaded = "DocumentUploaded"

	AggregateDocument = "document"

	JobTypeParseDocument = "parse_document"
)

// OutboxStatus is the publication lifecycle of an outbox event.
type OutboxStatus string

const (
	OutboxPending   OutboxStatus = "pending"
	OutboxPublished OutboxStatus = "published"
	OutboxFailed    OutboxStatus = "failed"
)

// JobStatus is the lifecycle of a processing job.
type JobStatus string

const (
	JobQueued         JobStatus = "queued"
	JobRunning        JobStatus = "running"
	JobRetryScheduled JobStatus = "retry_scheduled"
	JobCompleted      JobStatus = "completed"
	JobFailed         JobStatus = "failed"
	JobDeadLetter     JobStatus = "dead_letter"
)

// OutboxEvent is written transactionally before publication to an external queue.
type OutboxEvent struct {
	ID            string
	EventType     string
	AggregateType string
	AggregateID   string
	Payload       map[string]any
	Status        OutboxStatus
	Attempts      int
	LastError     string
	CreatedAt     time.Time
	PublishedAt   *time.Time
}

// Job is a retryable, idempotent background work item.
type Job struct {
	ID             string
	JobType        string
	AggregateType  string
	AggregateID    string
	Status         JobStatus
	Payload        map[string]any
	IdempotencyKey string
	Attempts       int
	MaxAttempts    int
	AvailableAt    time.Time
	LockedBy       string
	LockedUntil    *time.Time
	LastError      string
	CreatedAt      time.Time
	UpdatedAt      time.Time
}

// ParseDocumentPayload is the payload persisted for parse_document jobs.
type ParseDocumentPayload struct {
	DocumentID string `json:"document_id"`
	JVID       string `json:"jv_id"`
	StorageKey string `json:"storage_key"`
}

// ParsedTable represents one extracted table from a parsed document.
type ParsedTable struct {
	Sheet string     `json:"sheet,omitempty"`
	Page  int        `json:"page,omitempty"`
	Rows  [][]string `json:"rows"`
}

// DocumentChunk is a text chunk ready for indexing. Embedding is optional.
type DocumentChunk struct {
	Index     int       `json:"index"`
	Content   string    `json:"content"`
	Embedding []float64 `json:"embedding,omitempty"`
}

// ParseResult is the normalized parsed output stored by the Go worker.
type ParseResult struct {
	DocumentID string          `json:"document_id,omitempty"`
	Filename   string          `json:"filename"`
	Pages      int             `json:"pages"`
	Text       string          `json:"text"`
	Markdown   string          `json:"markdown"`
	Tables     []ParsedTable   `json:"tables"`
	Chunks     []DocumentChunk `json:"chunks,omitempty"`
	ParsedAt   time.Time       `json:"parsed_at,omitempty"`
}

// ParseResultSummary is a lightweight status view for parsed artifacts.
type ParseResultSummary struct {
	DocumentID      string     `json:"document_id"`
	Filename        string     `json:"filename"`
	Pages           int        `json:"pages"`
	TextBytes       int        `json:"text_bytes"`
	MarkdownBytes   int        `json:"markdown_bytes"`
	TablesCount     int        `json:"tables_count"`
	ChunksCount     int        `json:"chunks_count"`
	LastParsedAt    *time.Time `json:"last_parsed_at,omitempty"`
	LastCompletedAt *time.Time `json:"last_completed_at,omitempty"`
}

// DecodeParseDocumentPayload converts the job JSON payload into a typed value.
func DecodeParseDocumentPayload(job Job) (ParseDocumentPayload, error) {
	payload, err := json.Marshal(job.Payload)
	if err != nil {
		return ParseDocumentPayload{}, fmt.Errorf("marshaling job payload: %w", err)
	}

	var parsed ParseDocumentPayload
	if err = json.Unmarshal(payload, &parsed); err != nil {
		return ParseDocumentPayload{}, fmt.Errorf("decoding parse document payload: %w", err)
	}
	if parsed.DocumentID == "" || parsed.StorageKey == "" {
		return ParseDocumentPayload{}, fmt.Errorf("parse document payload is missing document_id or storage_key")
	}

	return parsed, nil
}

// NewDocumentUploadedOutboxEvent describes a document ready for async processing.
func NewDocumentUploadedOutboxEvent(documentID, jvID, storageKey string) OutboxEvent {
	now := time.Now().UTC()

	return OutboxEvent{
		ID:            uuid.NewString(),
		EventType:     EventDocumentUploaded,
		AggregateType: AggregateDocument,
		AggregateID:   documentID,
		Payload: map[string]any{
			"document_id": documentID,
			"jv_id":       jvID,
			"storage_key": storageKey,
		},
		Status:    OutboxPending,
		CreatedAt: now,
	}
}

// NewParseDocumentJob creates the MVP PostgreSQL-backed parsing job.
func NewParseDocumentJob(documentID, jvID, storageKey string) Job {
	now := time.Now().UTC()

	return Job{
		ID:            uuid.NewString(),
		JobType:       JobTypeParseDocument,
		AggregateType: AggregateDocument,
		AggregateID:   documentID,
		Status:        JobQueued,
		Payload: map[string]any{
			"document_id": documentID,
			"jv_id":       jvID,
			"storage_key": storageKey,
		},
		IdempotencyKey: "parse_document:" + documentID + ":v1",
		MaxAttempts:    5,
		AvailableAt:    now,
		CreatedAt:      now,
		UpdatedAt:      now,
	}
}
