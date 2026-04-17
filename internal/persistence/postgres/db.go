// Package postgres provides PostgreSQL-backed implementations of the
// persistence repository interfaces defined in the parent package.
// The pgx/v5 driver is used throughout, as specified in ADR-004.
package postgres

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/Mewtos7/lx-container-weaver/internal/persistence"
)

// Querier is satisfied by both *pgxpool.Pool and pgx.Tx, allowing repository
// methods to execute queries inside or outside a transaction without branching.
type Querier interface {
	Query(ctx context.Context, sql string, args ...any) (pgx.Rows, error)
	QueryRow(ctx context.Context, sql string, args ...any) pgx.Row
	Exec(ctx context.Context, sql string, args ...any) (pgconn.CommandTag, error)
}

// Open creates a new *pgxpool.Pool for the given DSN and verifies the
// connection is reachable with a ping.
func Open(ctx context.Context, dsn string) (*pgxpool.Pool, error) {
	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		return nil, fmt.Errorf("postgres: open pool: %w", err)
	}
	if err := pool.Ping(ctx); err != nil {
		pool.Close()
		return nil, fmt.Errorf("postgres: ping: %w", err)
	}
	return pool, nil
}

// WithTx executes fn inside a single database transaction. The transaction is
// automatically committed on success or rolled back on any error returned by fn.
func WithTx(ctx context.Context, pool *pgxpool.Pool, fn func(pgx.Tx) error) error {
	tx, err := pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("postgres: begin transaction: %w", err)
	}
	defer tx.Rollback(ctx) //nolint:errcheck

	if err := fn(tx); err != nil {
		return err
	}
	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("postgres: commit transaction: %w", err)
	}
	return nil
}

// mapErr converts pgx-specific errors to the sentinel errors defined in the
// persistence package so callers can inspect failures without importing pgx.
func mapErr(err error) error {
	if err == nil {
		return nil
	}
	if errors.Is(err, pgx.ErrNoRows) {
		return persistence.ErrNotFound
	}
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) {
		switch pgErr.Code {
		case "23505": // unique_violation
			return fmt.Errorf("%w: %s", persistence.ErrConflict, pgErr.Detail)
		case "23503": // foreign_key_violation
			return fmt.Errorf("%w: %s", persistence.ErrConflict, pgErr.Detail)
		case "23502": // not_null_violation
			return fmt.Errorf("%w: %s", persistence.ErrValidation, pgErr.Detail)
		case "23514": // check_violation
			return fmt.Errorf("%w: %s", persistence.ErrValidation, pgErr.Detail)
		}
	}
	return err
}
