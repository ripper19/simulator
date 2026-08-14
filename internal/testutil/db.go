// Package testutil provides helpers for tests that require a live PostgreSQL
// database. Each test is isolated in its own throwaway schema so tests can run
// in parallel without interfering.
package testutil

import (
	"context"
	"os"
	"strconv"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// TestPool returns a connection pool scoped to a unique, throwaway schema plus
// a cleanup function. If DATABASE_URL is unset or unreachable it skips the test
// and returns ok=false. Each pooled connection is configured to use the schema
// via search_path, so unqualified DDL/DML (including migrations) is isolated.
func TestPool(t *testing.T) (pool *pgxpool.Pool, cleanup func(), ok bool) {
	t.Helper()
	url := os.Getenv("DATABASE_URL")
	if url == "" {
		t.Skip("DATABASE_URL not set; skipping integration test")
		return nil, nil, false
	}

	schema := "test_" + strconv.FormatInt(time.Now().UnixNano(), 36)

	cfg, err := pgxpool.ParseConfig(url)
	if err != nil {
		t.Skipf("invalid DATABASE_URL: %v", err)
		return nil, nil, false
	}
	cfg.MaxConns = 4
	cfg.AfterConnect = func(ctx context.Context, conn *pgx.Conn) error {
		_, err := conn.Exec(ctx, "SET search_path TO "+schema)
		return err
	}

	pool, err = pgxpool.NewWithConfig(context.Background(), cfg)
	if err != nil {
		t.Skipf("connect: %v", err)
		return nil, nil, false
	}
	if err := pool.Ping(context.Background()); err != nil {
		pool.Close()
		t.Skipf("ping: %v", err)
		return nil, nil, false
	}
	if _, err := pool.Exec(context.Background(), "CREATE SCHEMA "+schema); err != nil {
		pool.Close()
		t.Skipf("create schema: %v", err)
		return nil, nil, false
	}

	cleanup = func() {
		_, _ = pool.Exec(context.Background(), "DROP SCHEMA IF EXISTS "+schema+" CASCADE")
		pool.Close()
	}
	return pool, cleanup, true
}
