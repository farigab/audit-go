// Package regions owns region concepts used to scope access.
package regions

import "time"

// Region groups joint ventures under an authorization boundary.
type Region struct {
	ID        string    `json:"id"`
	Name      string    `json:"name"`
	Code      string    `json:"code"`
	CreatedAt time.Time `json:"created_at"`
}
