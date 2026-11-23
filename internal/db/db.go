// Package db opens Postgres connection pools for the metadata DB and shards.
package db

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

// DefaultConnectTimeout bounds how long a single connection attempt may take
// when the DSN does not set connect_timeout itself.
const DefaultConnectTimeout = 5 * time.Second

// ErrEmptyDSN is returned for an empty DSN. pgx would otherwise silently fall
// back to libpq defaults (local socket, PG* environment variables).
var ErrEmptyDSN = errors.New("db: empty dsn")

// NewPool opens a pool of at most maxConns connections to dsn and pings it so
// a bad DSN or an unreachable database fails here, not on first use. The
// caller owns the returned pool and must Close it.
func NewPool(ctx context.Context, dsn string, maxConns int32) (*pgxpool.Pool, error) {
	if dsn == "" {
		return nil, ErrEmptyDSN
	}
	if maxConns <= 0 {
		return nil, fmt.Errorf("db: maxConns must be positive, got %d", maxConns)
	}
	cfg, err := pgxpool.ParseConfig(dsn)
	if err != nil {
		// pgx redacts the password in parse errors.
		return nil, fmt.Errorf("db: parse dsn: %w", err)
	}
	cfg.MaxConns = maxConns
	if cfg.ConnConfig.ConnectTimeout == 0 {
		cfg.ConnConfig.ConnectTimeout = DefaultConnectTimeout
	}
	target := fmt.Sprintf("%s:%d/%s", cfg.ConnConfig.Host, cfg.ConnConfig.Port, cfg.ConnConfig.Database)

	pool, err := pgxpool.NewWithConfig(ctx, cfg)
	if err != nil {
		return nil, fmt.Errorf("db: create pool for %s: %w", target, err)
	}
	if err := pool.Ping(ctx); err != nil {
		pool.Close()
		return nil, fmt.Errorf("db: ping %s: %w", target, err)
	}
	return pool, nil
}
