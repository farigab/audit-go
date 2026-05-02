// Package processing owns asynchronous job and outbox concepts.
package processing

import (
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
