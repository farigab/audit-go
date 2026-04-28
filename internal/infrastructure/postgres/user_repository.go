// Package postgres provides PostgreSQL repository implementations.
package postgres

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	"audit-go/internal/domain"
)

// UserRepository is a PostgreSQL-backed user repository.
type UserRepository struct {
	db *sql.DB
}

// NewUserRepository creates a PostgreSQL-backed user repository.
func NewUserRepository(db *sql.DB) *UserRepository {
	return &UserRepository{db: db}
}

// FindByLogin returns a user by login.
func (r *UserRepository) FindByLogin(ctx context.Context, login string) (*domain.User, error) {
	const query = `
		SELECT login, name, created_at
		FROM users
		WHERE login = $1
	`

	row := r.db.QueryRowContext(ctx, query, login)

	var user domain.User

	err := row.Scan(
		&user.Login,
		&user.Name,
		&user.CreatedAt,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, errors.New("user not found")
	}
	if err != nil {
		return nil, fmt.Errorf("querying user by login: %w", err)
	}

	user.CreatedAt = user.CreatedAt.UTC()

	return &user, nil
}

// Save inserts or updates a user (upsert on login).
func (r *UserRepository) Save(ctx context.Context, user *domain.User) error {
	const query = `
		INSERT INTO users (login, name)
		VALUES ($1, $2)
		ON CONFLICT (login) DO UPDATE SET
			name = EXCLUDED.name
	`

	_, err := r.db.ExecContext(ctx, query, user.Login, user.Name)
	if err != nil {
		return fmt.Errorf("saving user: %w", err)
	}

	return nil
}

// DeleteByLogin removes a user by login.
func (r *UserRepository) DeleteByLogin(ctx context.Context, login string) error {
	const query = `DELETE FROM users WHERE login = $1`

	res, err := r.db.ExecContext(ctx, query, login)
	if err != nil {
		return fmt.Errorf("deleting user: %w", err)
	}

	n, err := res.RowsAffected()
	if err != nil {
		return fmt.Errorf("checking rows affected: %w", err)
	}
	if n == 0 {
		return errors.New("user not found")
	}

	return nil
}

// Exists checks whether a user with the given login exists.
func (r *UserRepository) Exists(ctx context.Context, login string) (bool, error) {
	const query = `SELECT EXISTS(SELECT 1 FROM users WHERE login = $1)`

	var exists bool
	if err := r.db.QueryRowContext(ctx, query, login).Scan(&exists); err != nil {
		return false, fmt.Errorf("checking user existence: %w", err)
	}

	return exists, nil
}

// List returns all users ordered by login. Password hashes are NOT returned.
func (r *UserRepository) List(ctx context.Context) ([]*domain.User, error) {
	const query = `
		SELECT login, name, created_at
		FROM users
		ORDER BY login
	`

	rows, err := r.db.QueryContext(ctx, query)
	if err != nil {
		return nil, fmt.Errorf("listing users: %w", err)
	}
	defer func() { _ = rows.Close() }()

	var users []*domain.User

	for rows.Next() {
		var user domain.User

		if err = rows.Scan(&user.Login, &user.Name, &user.CreatedAt); err != nil {
			return nil, fmt.Errorf("scanning user row: %w", err)
		}

		user.CreatedAt = user.CreatedAt.UTC()
		users = append(users, &user)
	}

	if err = rows.Err(); err != nil {
		return nil, fmt.Errorf("iterating user rows: %w", err)
	}

	return users, nil
}
