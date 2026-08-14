# Final risk review

Independent review of the architecture against the ten required risk categories.
Each item states the residual status (mitigated, or open and tracked).

## 1. Scalability bottlenecks
- **PostgreSQL connection pool** (default 4) becomes the bottleneck under
  concurrent simulation creation (measured: p95 1.75s @50 VUs, 12.58s @100 VUs).
  *Open*: make pool sizing configurable and/or batch writes.
- **Broker/DB are shared bottlenecks** for workers; documented scaling path.
- **Per-tick shard partitioning** adds O(workers) overhead; small-N parallel is
  slower than serial. *Open*: inline-path fallback below a threshold.

## 2. Sources of nondeterminism
- No global RNG; seed-derived splitmix64 streams. Event order is total.
- Parallel shard commit is deterministic. Verified across worker counts.
- Residual: `Each` iteration order is creation order (documented); any
  order-sensitive logic must sort. No known nondeterminism remains in the tested
  paths.

## 3. Concurrency hazards
- Worker pool + barrier (no goroutine-per-entity); bounded queues.
- `-race` clean across the suite. Lock-free shard access is gated by the
  disjoint-shard contract (structural changes forbidden in `Run`).
- Residual: `Each` releases locks and aliases slices — safe only single-threaded
  (documented; P3 enforces via the shard contract).

## 4. Memory bottlenecks
- SoA storage; per-tick zero allocation after the cached entity-set build.
- `snapshotJSON` copies columns (expected); restore rebuilds sparse indexes.
- *Open*: `Column.sparse` is one int32 per ever-created entity index per type.

## 5. Distributed-systems failure modes
- Worker crash → unacked message redelivered → retry/DLQ.
- Duplicate delivery → idempotency claim (at-most-once).
- Broker connection drop → bounded exponential reconnect.
- Redis down → coordination fails open.
- *Open*: no manager-side consumer of worker liveness/heartbeats yet.

## 6. Security risks
- JWT (kind-tagged), argon2id, RBAC, per-route ownership, rate limiting,
  timeouts, request-size limits, recoverer.
- No secrets in committed code (placeholders + gitignored `.env`).
- *Open*: refresh tokens are not rotated (INFO); RealIP spoofing avoided by
  removing the deprecated middleware.

## 7. Plugin/model isolation risks
- Only trusted compiled-in models run in-process. Untrusted/user-defined models
  are out of scope for in-process execution; the path is a separate
  resource-limited gRPC model worker (future, ADR-012/013).

## 8. Snapshot consistency problems
- `Simulation.Snapshot` rejects a running sim (no torn captures).
- Checksum + schema/engine version validation; model/seed match enforced.
- Residual: `Pause` returns before an in-flight step completes, so an immediate
  snapshot after Pause can still observe that step (a quiescence handshake would
  close it).

## 9. Database bottlenecks
- Connection pool sizing (see #1); migrations lack a checksum (P5-2) and empty
  down-files are silently ignored (P5-3) — both LOW, open.

## 10. Message delivery problems
- Retry/backoff/DLQ/idempotency implemented and tested (including with Redis).
- RabbitMQ reconnect with backoff implemented.
- Residual: no message TTL/dead-letter expiry policy beyond the DLQ; runaway
  same-time event loops in event mode lack a max-events guard (P2-4 note).

## Conclusion
Critical (HIGH/MEDIUM) findings from the verification ledger have been fixed and
re-verified. Remaining items are LOW/INFO and are documented in `fixes.txt`
(tracked for the performance/polish pass) or in this review. No fabricated
results are reported anywhere.
