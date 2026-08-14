package database

import (
	"context"
	"fmt"
	"sort"
	"strconv"
	"strings"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/ripper19/simulator/migrations"
)

// Migration is a single versioned migration with up and down SQL.
type Migration struct {
	Version int
	Name    string
	Up      string
	Down    string
}

// LoadMigrations reads the embedded migration files and returns them ordered by
// version. Files must be named NNNN_name.up.sql / NNNN_name.down.sql.
func LoadMigrations() ([]Migration, error) {
	entries, err := migrations.FS.ReadDir(".")
	if err != nil {
		return nil, fmt.Errorf("read migrations: %w", err)
	}
	byVersion := map[int]*Migration{}
	for _, e := range entries {
		version, name, direction, err := parseName(e.Name())
		if err != nil {
			return nil, err
		}
		m := byVersion[version]
		if m == nil {
			m = &Migration{Version: version, Name: name}
			byVersion[version] = m
		}
		content, err := migrations.FS.ReadFile(e.Name())
		if err != nil {
			return nil, fmt.Errorf("read migration %s: %w", e.Name(), err)
		}
		if direction == "up" {
			m.Up = string(content)
		} else {
			m.Down = string(content)
		}
	}

	out := make([]Migration, 0, len(byVersion))
	for _, m := range byVersion {
		if m.Up == "" {
			return nil, fmt.Errorf("migration %d (%s) missing up file", m.Version, m.Name)
		}
		out = append(out, *m)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Version < out[j].Version })
	return out, nil
}

func parseName(filename string) (int, string, string, error) {
	if !strings.HasSuffix(filename, ".sql") {
		return 0, "", "", fmt.Errorf("unexpected migration file %q", filename)
	}
	stem := strings.TrimSuffix(filename, ".sql")
	dot := strings.LastIndex(stem, ".")
	if dot < 0 {
		return 0, "", "", fmt.Errorf("unexpected migration file %q", filename)
	}
	direction := stem[dot+1:]
	rest := stem[:dot]
	us := strings.Index(rest, "_")
	if us < 0 {
		return 0, "", "", fmt.Errorf("unexpected migration file %q", filename)
	}
	version, err := strconv.Atoi(rest[:us])
	if err != nil {
		return 0, "", "", fmt.Errorf("unexpected migration version in %q", filename)
	}
	return version, rest[us+1:], direction, nil
}

// Migrate applies all pending migrations in order, each in its own transaction.
func Migrate(ctx context.Context, pool *pgxpool.Pool) error {
	ms, err := LoadMigrations()
	if err != nil {
		return err
	}
	if _, err := pool.Exec(ctx, `
		CREATE TABLE IF NOT EXISTS schema_migrations (
			version    INT         PRIMARY KEY,
			name       TEXT        NOT NULL,
			applied_at TIMESTAMPTZ NOT NULL DEFAULT now()
		)`); err != nil {
		return fmt.Errorf("create schema_migrations: %w", err)
	}

	applied := map[int]bool{}
	rows, err := pool.Query(ctx, `SELECT version FROM schema_migrations`)
	if err != nil {
		return fmt.Errorf("read schema_migrations: %w", err)
	}
	for rows.Next() {
		var v int
		if err := rows.Scan(&v); err != nil {
			rows.Close()
			return err
		}
		applied[v] = true
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return err
	}

	for _, m := range ms {
		if applied[m.Version] {
			continue
		}
		if err := applyOne(ctx, pool, m, true); err != nil {
			return err
		}
	}
	return nil
}

// Rollback undoes the most recent `steps` applied migrations.
func Rollback(ctx context.Context, pool *pgxpool.Pool, steps int) error {
	ms, err := LoadMigrations()
	if err != nil {
		return err
	}
	applied := []Migration{}
	rows, err := pool.Query(ctx, `SELECT version FROM schema_migrations ORDER BY version DESC`)
	if err != nil {
		return err
	}
	var versions []int
	for rows.Next() {
		var v int
		if err := rows.Scan(&v); err != nil {
			rows.Close()
			return err
		}
		versions = append(versions, v)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return err
	}
	for i := 0; i < steps && i < len(versions); i++ {
		for _, m := range ms {
			if m.Version == versions[i] {
				applied = append(applied, m)
			}
		}
	}
	for _, m := range applied {
		if err := applyOne(ctx, pool, m, false); err != nil {
			return err
		}
	}
	return nil
}

func applyOne(ctx context.Context, pool *pgxpool.Pool, m Migration, up bool) error {
	tx, err := pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin migration %d: %w", m.Version, err)
	}
	defer tx.Rollback(ctx)

	if up {
		if _, err := tx.Exec(ctx, m.Up); err != nil {
			return fmt.Errorf("apply migration %d (%s): %w", m.Version, m.Name, err)
		}
		if _, err := tx.Exec(ctx, `INSERT INTO schema_migrations (version, name) VALUES ($1, $2)`, m.Version, m.Name); err != nil {
			return fmt.Errorf("record migration %d: %w", m.Version, err)
		}
	} else {
		if _, err := tx.Exec(ctx, m.Down); err != nil {
			return fmt.Errorf("rollback migration %d (%s): %w", m.Version, m.Name, err)
		}
		if _, err := tx.Exec(ctx, `DELETE FROM schema_migrations WHERE version = $1`, m.Version); err != nil {
			return fmt.Errorf("unrecord migration %d: %w", m.Version, err)
		}
	}
	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("commit migration %d: %w", m.Version, err)
	}
	return nil
}
