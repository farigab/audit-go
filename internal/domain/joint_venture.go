package domain

import (
	"errors"
	"time"
)

// JVStatus describes the lifecycle state of a JointVenture.
type JVStatus string

// JVStatus constants represent known lifecycle states for a joint venture.
const (
	JVStatusDraft     JVStatus = "draft"
	JVStatusActive    JVStatus = "active"
	JVStatusSuspended JVStatus = "suspended"
	JVStatusClosed    JVStatus = "closed"
)

// JointVenture represents a collaboration between parties.
type JointVenture struct {
	ID        string            `json:"id"`
	Name      string            `json:"name"`
	Parties   []string          `json:"parties"`
	Status    JVStatus          `json:"status"`
	CreatedBy string            `json:"created_by"`
	CreatedAt time.Time         `json:"created_at"`
	UpdatedAt time.Time         `json:"updated_at"`
	Metadata  map[string]string `json:"metadata,omitempty"`
}

// NewJointVenture creates a new JointVenture, validating required fields.
func NewJointVenture(id, name, createdBy string, parties []string) (JointVenture, error) {
	if name == "" {
		return JointVenture{}, errors.New("joint venture name is required")
	}
	if len(parties) < 2 {
		return JointVenture{}, errors.New("a joint venture requires at least two parties")
	}

	now := time.Now().UTC()

	return JointVenture{
		ID:        id,
		Name:      name,
		Parties:   parties,
		Status:    JVStatusDraft,
		CreatedBy: createdBy,
		CreatedAt: now,
		UpdatedAt: now,
		Metadata:  make(map[string]string),
	}, nil
}

// Activate sets the joint venture to active when in draft state.
func (jv *JointVenture) Activate() error {
	if jv.Status != JVStatusDraft {
		return errors.New("only draft joint ventures can be activated")
	}
	jv.Status = JVStatusActive
	jv.UpdatedAt = time.Now().UTC()
	return nil
}

// Suspend sets the joint venture to suspended when in active state.
func (jv *JointVenture) Suspend() error {
	if jv.Status != JVStatusActive {
		return errors.New("only active joint ventures can be suspended")
	}
	jv.Status = JVStatusSuspended
	jv.UpdatedAt = time.Now().UTC()
	return nil
}

// IsActive reports whether the joint venture is currently active.
func (jv JointVenture) IsActive() bool {
	return jv.Status == JVStatusActive
}
