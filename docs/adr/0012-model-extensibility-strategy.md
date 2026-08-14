# ADR-012: Model extensibility strategy

## Status
Accepted

## Context
Models must be easy to add and version, without compromising determinism,
security, or portability. Options: compiled-in interfaces, Go plugins, WASM,
out-of-process gRPC.

## Decision
Compiled-in Go interfaces as the primary path, with an out-of-process gRPC
worker as the future path for untrusted models. Go's `plugin` package and WASM
are rejected for now.

## Consequences
- Compiled-in models (`simulation.Model` + `TickModel`/`EventModel`/
  `SystemModel`/`ConfigurableModel`/`SnapshotModel`) are type-safe, fast, and
  deterministic, and they register in the `registry` with versioned metadata.
- The registry persists model metadata to PostgreSQL and pins the version a
  simulation ran with, so reproducibility survives newer model releases.
- Trade-off: adding a model requires a rebuild/deploy of the worker binary. For
  untrusted/dynamic models, run them as separate gRPC processes (isolation) —
  deferred, tracked in ADR-013.
- Go plugins rejected: fragile (no cross-version compatibility, requires
  identical build flags). WASM rejected: adds a runtime and toolchain without a
  current requirement.
