// Package prompts owns prompt management and audit chatbot concepts.
package prompts

import "time"

type VersionStatus string

const (
	VersionDraft      VersionStatus = "draft"
	VersionApproved   VersionStatus = "approved"
	VersionDeprecated VersionStatus = "deprecated"
)

type Prompt struct {
	ID              string    `json:"id"`
	Name            string    `json:"name"`
	Description     string    `json:"description"`
	Category        string    `json:"category"`
	ActiveVersionID string    `json:"active_version_id,omitempty"`
	CreatedBy       string    `json:"created_by"`
	CreatedAt       time.Time `json:"created_at"`
	UpdatedAt       time.Time `json:"updated_at"`
}

type Version struct {
	ID           string        `json:"id"`
	PromptID     string        `json:"prompt_id"`
	Version      int           `json:"version"`
	SystemPrompt string        `json:"system_prompt"`
	UserTemplate string        `json:"user_template"`
	Model        string        `json:"model"`
	Temperature  float64       `json:"temperature"`
	Status       VersionStatus `json:"status"`
	CreatedBy    string        `json:"created_by"`
	CreatedAt    time.Time     `json:"created_at"`
	ApprovedBy   string        `json:"approved_by,omitempty"`
	ApprovedAt   *time.Time    `json:"approved_at,omitempty"`
	DeprecatedAt *time.Time    `json:"deprecated_at,omitempty"`
}

type Run struct {
	ID              string    `json:"id"`
	PromptVersionID string    `json:"prompt_version_id,omitempty"`
	JVID            string    `json:"jv_id"`
	Question        string    `json:"question"`
	Answer          string    `json:"answer"`
	ContextBytes    int       `json:"context_bytes"`
	CreatedBy       string    `json:"created_by"`
	CreatedAt       time.Time `json:"created_at"`
}
