// Package domain defines the core domain entities used by the audit service.
package domain

import "time"

// Action descreve o que aconteceu — sempre no passado.
type Action string

// Action constants represent audit event action types.
const (
	ActionJVCreated        Action = "jv.created"
	ActionJVActivated      Action = "jv.activated"
	ActionJVSuspended      Action = "jv.suspended"
	ActionDocumentUploaded Action = "document.uploaded"
	ActionDocumentDeleted  Action = "document.deleted"
	ActionDocumentParsed   Action = "document.parsed" // Python terminou o processamento
	ActionChatQueried      Action = "chat.queried"    // usuário fez pergunta ao AI
)

// TargetType identifica sobre qual entidade o evento se refere.
type TargetType string

// TargetType constants represent entity types for audit events.
const (
	TargetJointVenture TargetType = "joint_venture"
	TargetDocument     TargetType = "document"
	TargetChat         TargetType = "chat"
)

// AuditEvent é imutável por design: nunca há Update, só Insert.
// Representa um fato que aconteceu no sistema — não pode ser desfeito.
type AuditEvent struct {
	ID         string            `json:"id"`
	ActorID    string            `json:"actor_id"` // user_id de quem fez a ação
	Action     Action            `json:"action"`
	TargetID   string            `json:"target_id"` // id da entidade afetada
	TargetType TargetType        `json:"target_type"`
	OccurredAt time.Time         `json:"occurred_at"`
	RequestID  string            `json:"request_id"`         // rastreabilidade HTTP
	Metadata   map[string]string `json:"metadata,omitempty"` // ip, user-agent, detalhes extras
}

// NewAuditEvent é a única forma de criar um evento — sem construtor alternativo.
func NewAuditEvent(
	id, actorID, requestID string,
	action Action,
	targetID string,
	targetType TargetType,
) AuditEvent {
	return AuditEvent{
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

// WithMetadata adiciona contexto extra ao evento (IP, filename, model usado etc).
// Retorna o próprio evento para permitir encadeamento.
func (e AuditEvent) WithMetadata(key, value string) AuditEvent {
	e.Metadata[key] = value
	return e
}
