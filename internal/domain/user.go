// Package domain contains domain models used across the application.
package domain

// User represents a user persisted in the application.
type User struct {
	Login             string `db:"login" json:"login"`
	Name              string `db:"name" json:"name"`
}

// NewUser creates a new User instance from the provided fields.
func NewUser(login, name string) *User {
	return &User{Login: login, Name: name}
}
