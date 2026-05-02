// Package audit owns immutable audit event concepts.
package audit

import "time"

// Action describes an event that happened in the system.
type Action string

const (
	ActionJVCreated        Action = "jv.created"
	ActionJVActivated      Action = "jv.activated"
	ActionJVSuspended      Action = "jv.suspended"
	ActionDocumentUploaded Action = "document.uploaded"
	ActionDocumentDeleted  Action = "document.deleted"
	ActionDocumentParsed   Action = "document.parsed"
	ActionChatQueried      Action = "chat.queried"
)

// TargetType identifies the entity affected by an event.
type TargetType string

const (
	TargetJointVenture TargetType = "joint_venture"
	TargetDocument     TargetType = "document"
	TargetChat         TargetType = "chat"
)

// Event is immutable by design: it is inserted once and never updated.
type Event struct {
	ID         string            `json:"id"`
	ActorID    string            `json:"actor_id"`
	Action     Action            `json:"action"`
	TargetID   string            `json:"target_id"`
	TargetType TargetType        `json:"target_type"`
	OccurredAt time.Time         `json:"occurred_at"`
	RequestID  string            `json:"request_id"`
	Metadata   map[string]string `json:"metadata,omitempty"`
}

// NewEvent creates an immutable audit event.
func NewEvent(
	id, actorID, requestID string,
	action Action,
	targetID string,
	targetType TargetType,
) Event {
	return Event{
		ID:         id,
		ActorID:    actorID,
		Action:     action,
		TargetID:   targetID,
		TargetType: targetType,
		OccurredAt: time.Now().UTC(),
		RequestID:  requestID,
		Metadata:   make(map[string]string),
	}
}

// WithMetadata adds context to the event and returns the updated value.
func (e Event) WithMetadata(key, value string) Event {
	e.Metadata[key] = value
	return e
}
