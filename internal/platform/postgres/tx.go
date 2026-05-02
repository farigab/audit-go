package postgres

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
)

type txKey struct{}

// DBTX is the subset of *sql.DB and *sql.Tx used by repositories.
type DBTX interface {
	ExecContext(ctx context.Context, query string, args ...any) (sql.Result, error)
	QueryContext(ctx context.Context, query string, args ...any) (*sql.Rows, error)
	QueryRowContext(ctx context.Context, query string, args ...any) *sql.Row
}

// Transactor runs repository operations inside a shared database transaction.
type Transactor struct {
	db *sql.DB
}

// NewTransactor creates a transaction runner backed by db.
func NewTransactor(db *sql.DB) Transactor {
	return Transactor{db: db}
}

// WithinTx runs fn inside a transaction and commits only if fn returns nil.
func (t Transactor) WithinTx(ctx context.Context, fn func(context.Context) error) error {
	if t.db == nil {
		return errors.New("postgres transactor has nil db")
	}

	if existing := TxFromContext(ctx); existing != nil {
		return fn(ctx)
	}

	tx, err := t.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("beginning transaction: %w", err)
	}

	txCtx := context.WithValue(ctx, txKey{}, tx)
	if err = fn(txCtx); err != nil {
		if rbErr := tx.Rollback(); rbErr != nil {
			return fmt.Errorf("rollback after %v: %w", err, rbErr)
		}
		return err
	}

	if err = tx.Commit(); err != nil {
		return fmt.Errorf("committing transaction: %w", err)
	}

	return nil
}

// TxFromContext returns the transaction stored in ctx, if any.
func TxFromContext(ctx context.Context) *sql.Tx {
	tx, _ := ctx.Value(txKey{}).(*sql.Tx)
	return tx
}

// Executor returns the active transaction for ctx or db otherwise.
func Executor(ctx context.Context, db *sql.DB) DBTX {
	if tx := TxFromContext(ctx); tx != nil {
		return tx
	}
	return db
}
