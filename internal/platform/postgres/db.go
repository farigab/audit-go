// Package postgres provides shared PostgreSQL infrastructure.
package postgres

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	_ "github.com/lib/pq" // register postgres driver
)

// ConnectionPoolConfig configures the sql.DB pool.
type ConnectionPoolConfig struct {
	MaxOpenConns    int
	MaxIdleConns    int
	ConnMaxLifetime time.Duration
	ConnMaxIdleTime time.Duration
}

// Connect opens and verifies a connection to Postgres using the provided DSN.
func Connect(dsn string) (*sql.DB, error) {
	return ConnectWithPool(dsn, ConnectionPoolConfig{
		MaxOpenConns:    25,
		MaxIdleConns:    5,
		ConnMaxLifetime: 5 * time.Minute,
		ConnMaxIdleTime: 2 * time.Minute,
	})
}

// ConnectWithPool opens and verifies a connection to Postgres using the provided DSN and pool settings.
func ConnectWithPool(dsn string, pool ConnectionPoolConfig) (*sql.DB, error) {
	db, err := sql.Open("postgres", dsn)
	if err != nil {
		return nil, fmt.Errorf("opening postgres connection: %w", err)
	}

	if err = db.PingContext(context.Background()); err != nil {
		return nil, fmt.Errorf("pinging postgres: %w", err)
	}

	if pool.MaxOpenConns <= 0 {
		pool.MaxOpenConns = 25
	}
	if pool.MaxIdleConns < 0 {
		pool.MaxIdleConns = 0
	}
	if pool.ConnMaxLifetime <= 0 {
		pool.ConnMaxLifetime = 5 * time.Minute
	}
	if pool.ConnMaxIdleTime < 0 {
		pool.ConnMaxIdleTime = 0
	}

	db.SetMaxOpenConns(pool.MaxOpenConns)
	db.SetMaxIdleConns(pool.MaxIdleConns)
	db.SetConnMaxLifetime(pool.ConnMaxLifetime)
	db.SetConnMaxIdleTime(pool.ConnMaxIdleTime)

	return db, nil
}
