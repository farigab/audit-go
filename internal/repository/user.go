// Package repository defines persistence contracts.
package repository

import (
	"context"

	"audit-go/internal/domain"
)

// UserRepository defines user storage operations.
type UserRepository interface {
	// FindByLogin returns a user by login.
	FindByLogin(ctx context.Context, login string) (*domain.User, error)

	// Save creates or updates a user.
	Save(ctx context.Context, user *domain.User) error

	// DeleteByLogin removes a user by login.
	DeleteByLogin(ctx context.Context, login string) error

	// Exists checks whether a user exists.
	Exists(ctx context.Context, login string) (bool, error)

	// List returns all users.
	List(ctx context.Context) ([]*domain.User, error)
}
