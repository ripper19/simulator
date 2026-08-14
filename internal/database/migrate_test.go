package database

import (
	"context"
	"testing"

	"github.com/ripper19/simulator/internal/testutil"
)

func TestParseName(t *testing.T) {
	cases := []struct {
		filename string
		version  int
		name     string
		dir      string
		wantErr  bool
	}{
		{"0001_init.up.sql", 1, "init", "up", false},
		{"0001_init.down.sql", 1, "init", "down", false},
		{"0042_add_things.up.sql", 42, "add_things", "up", false},
		{"bad.sql", 0, "", "", true},
	}
	for _, c := range cases {
		v, n, d, err := parseName(c.filename)
		if c.wantErr {
			if err == nil {
				t.Errorf("%s: expected error", c.filename)
			}
			continue
		}
		if err != nil {
			t.Errorf("%s: %v", c.filename, err)
			continue
		}
		if v != c.version || n != c.name || d != c.dir {
			t.Errorf("%s: got (%d,%q,%q), want (%d,%q,%q)", c.filename, v, n, d, c.version, c.name, c.dir)
		}
	}
}

func TestLoadMigrations(t *testing.T) {
	ms, err := LoadMigrations()
	if err != nil {
		t.Fatal(err)
	}
	if len(ms) == 0 {
		t.Fatal("no migrations loaded")
	}
	if ms[0].Version != 1 {
		t.Fatalf("first migration version = %d, want 1", ms[0].Version)
	}
	for _, m := range ms {
		if m.Up == "" || m.Down == "" {
			t.Fatalf("migration %d (%s) missing up/down", m.Version, m.Name)
		}
	}
}

func TestMigrateUpAndDown(t *testing.T) {
	pool, cleanup, ok := testutil.TestPool(t)
	if !ok {
		return
	}
	defer cleanup()
	ctx := context.Background()

	if err := Migrate(ctx, pool); err != nil {
		t.Fatalf("migrate up: %v", err)
	}

	var count int
	err := pool.QueryRow(ctx, `
		SELECT count(*) FROM information_schema.tables
		WHERE table_schema = current_schema()
		  AND table_name IN ('models','simulations','snapshots','schema_migrations')`).Scan(&count)
	if err != nil {
		t.Fatal(err)
	}
	if count != 4 {
		t.Fatalf("expected 4 tables, found %d", count)
	}

	// Migrate is idempotent.
	if err := Migrate(ctx, pool); err != nil {
		t.Fatalf("migrate up (idempotent): %v", err)
	}

	ms, err := LoadMigrations()
	if err != nil {
		t.Fatal(err)
	}
	if err := Rollback(ctx, pool, len(ms)); err != nil {
		t.Fatalf("rollback: %v", err)
	}
	err = pool.QueryRow(ctx, `
		SELECT count(*) FROM information_schema.tables
		WHERE table_schema = current_schema()
		  AND table_name IN ('models','simulations','snapshots')`).Scan(&count)
	if err != nil {
		t.Fatal(err)
	}
	if count != 0 {
		t.Fatalf("expected 0 tables after rollback, found %d", count)
	}
}
