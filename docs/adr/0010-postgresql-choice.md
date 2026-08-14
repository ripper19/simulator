# ADR-010: PostgreSQL choice

## Status
Accepted

## Context
Durable metadata and state (models, simulations, users, snapshots) need a
relational store with strong consistency and migrations.

## Decision
Use PostgreSQL 17 via `pgx/v5` + `sqlc` (no ORM), with embedded SQL migrations
applied by a self-contained runner (`schema_migrations`).

## Consequences
- Type-safe generated queries with explicit SQL keep behavior transparent and
  avoid ORM impedance.
- Foreign keys, constraints, and indexes (model versioning, ownership) enforce
  integrity at the DB layer.
- Snapshots are stored as JSONB; JSON canonicalization means snapshots are
  compared semantically, not byte-for-byte, when read back.
- Migrations are embedded in the binary, so `cmd/migrate` and the API (which
  auto-migrates) work without external files.
