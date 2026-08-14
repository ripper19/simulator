// Package migrations embeds the SQL migration files so they can be shipped
// with the binary and applied by the migration runner without external files.
package migrations

import "embed"

// FS holds the migration SQL files (NNNN_name.up.sql / .down.sql).
//
//go:embed *.sql
var FS embed.FS
