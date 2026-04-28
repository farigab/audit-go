// Package postgres provides helpers for connecting to a Postgres database and basic repositories.
package postgres

import (
	"context"
	"database/sql"
	"fmt"

	_ "github.com/lib/pq" // register postgres driver
)

// Connect opens and verifies a connection to Postgres using the provided DSN.
func Connect(dsn string) (*sql.DB, error) {
	db, err := sql.Open("postgres", dsn)
	if err != nil {
		return nil, fmt.Errorf("opening postgres connection: %w", err)
	}

	if err := db.PingContext(context.Background()); err != nil {
		return nil, fmt.Errorf("pinging postgres: %w", err)
	}

	db.SetMaxOpenConns(25)
	db.SetMaxIdleConns(5)

	return db, nil
}
