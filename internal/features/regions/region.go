// Package regions owns region concepts used to scope access.
package regions

import (
	"errors"
	"strings"
	"time"
)

// Region groups joint ventures under an authorization boundary.
type Region struct {
	ID        string    `json:"id"`
	Name      string    `json:"name"`
	Code      string    `json:"code"`
	CreatedAt time.Time `json:"created_at"`
}

// New creates a region with normalized code.
func New(id, name, code string) (Region, error) {
	name = strings.TrimSpace(name)
	code = NormalizeCode(code)
	if id == "" || name == "" || code == "" {
		return Region{}, errors.New("region id, name, and code are required")
	}

	return Region{
		ID:        id,
		Name:      name,
		Code:      code,
		CreatedAt: time.Now().UTC(),
	}, nil
}

// NormalizeCode returns the canonical representation for a region code.
func NormalizeCode(code string) string {
	return strings.ToUpper(strings.TrimSpace(code))
}
