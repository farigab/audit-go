// Package domain contains domain models used across the application.
package domain

import (
	"errors"
	"time"
)

// User represents a user persisted in the application.
type User struct {
	Login     string    `db:"login"         json:"login"`
	Name      string    `db:"name"          json:"name"`
	CreatedAt time.Time `db:"created_at"    json:"created_at"`
}

// NewUser creates a new User.
func NewUser(login, name string) (*User, error) {
	if login == "" {
		return nil, errors.New("login is required")
	}
	if name == "" {
		return nil, errors.New("name is required")
	}

	return &User{
		Login:     login,
		Name:      name,
		CreatedAt: time.Now().UTC(),
	}, nil
}
