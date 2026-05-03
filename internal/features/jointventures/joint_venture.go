// Package jointventures owns joint venture domain concepts.
package jointventures

import (
	"errors"
	"strings"
	"time"
)

// Status describes the lifecycle state of a joint venture.
type Status string

const (
	StatusDraft     Status = "draft"
	StatusActive    Status = "active"
	StatusSuspended Status = "suspended"
	StatusClosed    Status = "closed"
)

// JointVenture represents a collaboration between parties.
type JointVenture struct {
	ID        string            `json:"id"`
	RegionID  string            `json:"region_id,omitempty"`
	Name      string            `json:"name"`
	Parties   []string          `json:"parties"`
	Status    Status            `json:"status"`
	CreatedBy string            `json:"created_by"`
	CreatedAt time.Time         `json:"created_at"`
	UpdatedAt time.Time         `json:"updated_at"`
	Metadata  map[string]string `json:"metadata,omitempty"`
}

// New creates a joint venture, validating required fields.
func New(id, regionID, name, createdBy string, parties []string) (JointVenture, error) {
	name = strings.TrimSpace(name)
	parties = CleanParties(parties)
	if name == "" {
		return JointVenture{}, errors.New("joint venture name is required")
	}
	if regionID == "" {
		return JointVenture{}, errors.New("joint venture region is required")
	}
	if len(parties) < 2 {
		return JointVenture{}, errors.New("a joint venture requires at least two parties")
	}

	now := time.Now().UTC()

	return JointVenture{
		ID:        id,
		RegionID:  regionID,
		Name:      name,
		Parties:   parties,
		Status:    StatusDraft,
		CreatedBy: createdBy,
		CreatedAt: now,
		UpdatedAt: now,
		Metadata:  make(map[string]string),
	}, nil
}

// Activate sets the joint venture to active when in draft state.
func (jv *JointVenture) Activate() error {
	if jv.Status != StatusDraft {
		return errors.New("only draft joint ventures can be activated")
	}
	jv.Status = StatusActive
	jv.UpdatedAt = time.Now().UTC()
	return nil
}

// Suspend sets the joint venture to suspended when in active state.
func (jv *JointVenture) Suspend() error {
	if jv.Status != StatusActive {
		return errors.New("only active joint ventures can be suspended")
	}
	jv.Status = StatusSuspended
	jv.UpdatedAt = time.Now().UTC()
	return nil
}

// IsActive reports whether the joint venture is currently active.
func (jv JointVenture) IsActive() bool {
	return jv.Status == StatusActive
}

// IsValidStatus reports whether status is supported.
func IsValidStatus(status Status) bool {
	switch status {
	case StatusDraft, StatusActive, StatusSuspended, StatusClosed:
		return true
	default:
		return false
	}
}

// NormalizeStatus returns draft when no status was supplied.
func NormalizeStatus(status Status) Status {
	if status == "" {
		return StatusDraft
	}
	return status
}

// CleanParties trims empty party names.
func CleanParties(parties []string) []string {
	cleaned := make([]string, 0, len(parties))
	for _, party := range parties {
		party = strings.TrimSpace(party)
		if party != "" {
			cleaned = append(cleaned, party)
		}
	}
	return cleaned
}
