// Package postgres provides PostgreSQL repository implementations.
package postgres

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	"audit-go/internal/domain"
)

type UserRepository struct {
	db *sql.DB
}

// NewUserRepository creates a PostgreSQL-backed user repository.
func NewUserRepository(db *sql.DB) *UserRepository {
	return &UserRepository{db: db}
}

// FindByLogin returns a user by login.
func (r *UserRepository) FindByLogin(ctx context.Context, login string) (*domain.User, error) {
	query := `
		SELECT login, name
		FROM users
		WHERE login = $1
	`

	row := r.db.QueryRowContext(ctx, query, login)

	var user domain.User

	err := row.Scan(
		&user.Login,
		&user.Name,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, errors.New("user not found")
	}
	if err != nil {
		return nil, fmt.Errorf("querying user by login: %w", err)
	}

	return &user, nil
}

// Save inserts or updates a user.
func (r *UserRepository) Save(ctx context.Context, user *domain.User) error {
	query := `
		INSERT INTO users (login, name)
		VALUES ($1, $2)
		ON CONFLICT (login) DO UPDATE SET
			name = EXCLUDED.name
	`

	_, err := r.db.ExecContext(
		ctx,
		query,
		user.Login,
		user.Name,
	)
	if err != nil {
		return fmt.Errorf("saving user: %w", err)
	}

	return nil
}

// DeleteByLogin removes a user by login.
func (r *UserRepository) DeleteByLogin(ctx context.Context, login string) error {
	query := `DELETE FROM users WHERE login = $1`

	res, err := r.db.ExecContext(ctx, query, login)
	if err != nil {
		return fmt.Errorf("deleting user: %w", err)
	}

	rows, err := res.RowsAffected()
	if err != nil {
		return fmt.Errorf("checking rows affected: %w", err)
	}

	if rows == 0 {
		return errors.New("user not found")
	}

	return nil
}

// Exists checks whether a user exists.
func (r *UserRepository) Exists(ctx context.Context, login string) (bool, error) {
	query := `
		SELECT EXISTS(
			SELECT 1
			FROM users
			WHERE login = $1
		)
	`

	var exists bool

	err := r.db.QueryRowContext(ctx, query, login).Scan(&exists)
	if err != nil {
		return false, fmt.Errorf("checking user existence: %w", err)
	}

	return exists, nil
}

// List returns all users ordered by login.
func (r *UserRepository) List(ctx context.Context) ([]*domain.User, error) {
	query := `
		SELECT login, name
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

		if err = rows.Scan(
			&user.Login,
			&user.Name,
		); err != nil {
			return nil, fmt.Errorf("scanning user row: %w", err)
		}

		users = append(users, &user)
	}

	if err = rows.Err(); err != nil {
		return nil, fmt.Errorf("iterating user rows: %w", err)
	}

	return users, nil
}
