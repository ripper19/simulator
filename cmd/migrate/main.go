// Command migrate applies (or rolls back) the embedded SQL migrations.
package main

import (
	"context"
	"flag"
	"fmt"
	"log/slog"
	"os"

	"github.com/ripper19/simulator/internal/database"
)

func main() {
	url := flag.String("url", os.Getenv("DATABASE_URL"), "PostgreSQL connection URL (defaults to $DATABASE_URL)")
	down := flag.Int("down", 0, "number of migrations to roll back (default 0 = migrate up)")
	flag.Parse()

	if *url == "" {
		fmt.Fprintln(os.Stderr, "error: no database URL; set DATABASE_URL or pass -url")
		os.Exit(2)
	}

	ctx := context.Background()
	pool, err := database.Open(ctx, *url)
	if err != nil {
		slog.Error("connect", "err", err)
		os.Exit(1)
	}
	defer pool.Close()

	if *down > 0 {
		if err := database.Rollback(ctx, pool, *down); err != nil {
			slog.Error("rollback", "err", err)
			os.Exit(1)
		}
		slog.Info("rolled back migrations", "count", *down)
		return
	}
	if err := database.Migrate(ctx, pool); err != nil {
		slog.Error("migrate", "err", err)
		os.Exit(1)
	}
	slog.Info("migrations applied")
}
