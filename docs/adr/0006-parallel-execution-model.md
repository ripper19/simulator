# ADR-006: Parallel execution model

## Status
Accepted

## Context
Independent work within a tick should parallelize without breaking determinism
or spawning goroutine-per-entity.

## Decision
- A model may decompose a tick into `System`s declaring `Reads`/`Writes`
  component IDs. The scheduler builds dependency levels (conflicting systems
  serialize; independent ones run concurrently).
- Within a system, entities are partitioned into contiguous shards of the dense
  component order (cache-friendly, no false sharing) and run on a bounded worker
  pool (`runConcurrent`). Each entity is written by exactly one shard.
- Shard-local access uses lock-free `GetShard`/`SetShard`; structural changes
  (create/destroy, component add/remove) are forbidden inside `Run`.

## Consequences
- Determinism holds for any worker count (verified: workers=1 vs 8 identical).
- Bounded concurrency: goroutine count is `systems × workers`, not `entities`.
- Trade-off: per-tick shard partitioning and the entity-set cache add a small
  overhead; small entity counts may be slower than serial (documented).
