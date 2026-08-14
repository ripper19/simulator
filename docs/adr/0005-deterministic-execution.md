# ADR-005: Deterministic execution

## Status
Accepted

## Context
A run must be reproducible given identical model, seed, configuration, and
execution parameters — including under parallel execution.

## Decision
- No package-level RNG. All randomness flows from one seed into splitmix64
  streams derived deterministically per concern (`Derive`/`DeriveU64`).
- Event ordering is a total order `(time, priority, sequence, id)`; sequence is
  assigned at scheduling time, so ordering depends only on scheduling order.
- The parallel scheduler commits shard writes in deterministic order and assigns
  shards deterministically, so results are independent of goroutine scheduling.
- Iteration that feeds effects is deterministic (creation order), and any
  order-sensitive logic sorts by ID.

## Consequences
- Same seed ⇒ same fingerprint, verified across worker counts and modes.
- Models must keep entity updates self-contained for parallel determinism (each
  entity's new state depends only on its own prior state + read-only shared
  state); the `System.Run` shard contract documents this.
- Cross-run reproducibility is a test-gated invariant (see counter/snapshot
  determinism tests).
