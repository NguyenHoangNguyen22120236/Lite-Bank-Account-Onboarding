// Package postgres provides connection lifecycle support without owning service data.
package postgres

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/jackc/pgx/v5/pgxpool"
)

// Open creates a pool with pgxpool defaults and verifies connectivity before
// returning. The caller supplies a context with an appropriate startup deadline
// and owns closing the returned pool. Open never runs migrations.
func Open(ctx context.Context, dsn string) (*pgxpool.Pool, error) {
	if strings.TrimSpace(dsn) == "" {
		return nil, errors.New("postgres: DSN is required")
	}
	config, err := pgxpool.ParseConfig(dsn)
	if err != nil {
		return nil, errors.New("postgres: invalid DSN")
	}
	pool, err := pgxpool.NewWithConfig(ctx, config)
	if err != nil {
		return nil, connectionError(ctx)
	}
	if err := pool.Ping(ctx); err != nil {
		pool.Close()
		return nil, connectionError(ctx)
	}
	return pool, nil
}

// Driver errors can contain DSNs, passwords, and server-supplied details.
// Only caller context errors are safe to wrap for errors.Is checks.
func connectionError(ctx context.Context) error {
	if err := ctx.Err(); err != nil {
		return fmt.Errorf("postgres: connection failed: %w", err)
	}
	return errors.New("postgres: connection failed")
}
