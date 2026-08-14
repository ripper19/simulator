# ADR-001: Why Go?

## Status
Accepted

## Context
The platform is a domain-agnostic simulation runtime that must be concurrent,
deterministic, and deployable both as a library, a CLI, an API, and a worker
cluster. Candidates considered: Go, Rust, Java/Kotlin, C++, Python.

## Decision
Use Go.

## Consequences
- Goroutines + channels give a simple, safe concurrency model (worker pools,
  bounded queues, context cancellation) without an async runtime.
- Single static binary simplifies CLI/worker/API deployment and containers.
- `go test -race ./...` provides a first-class data-race detector, which the
  parallel-determinism requirements depend on.
- Generics (1.18+) enable type-safe SoA component columns without codegen.
- Trade-off: no manual SIMD/memory layout control; performance tuning is
  profile-driven (deferred to the performance phase).
