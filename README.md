# simulator

An open, general-purpose distributed simulation runtime written in Go for
executing deterministic, extensible simulations locally or across a scalable
worker cluster.

> **Status: Phase 4** — core engine (entities, SoA components, events, clock,
> deterministic randomness, and the simulation runtime), the discrete-event
> engine (priority queue, immediate/delayed/prioritized scheduling), deterministic
> parallel execution (`System` abstraction with dependency ordering and sharded
> workers), and versioned, checksummed snapshots with deterministic restore.
> Later phases add persistence, REST API, distributed workers, and observability.

## What this is

A domain-agnostic simulation platform. The engine provides generic primitives —
entities, components, resources, tags, events, a clock, and deterministic random
streams — from which any model builds its own domain concepts (traffic,
ecosystem, queueing, logistics, economics, …). The engine itself knows nothing
about any of those domains.

## What this is not

It is not a domain-specific simulator, and it does not simulate "anything"
without qualification. It executes models written against its `Model` interface,
in discrete-tick or discrete-event mode, deterministically.

## Layout

```
pkg/rng         deterministic, seedable RNG with stream derivation
pkg/model       model metadata + execution mode (leaf, no engine deps)
pkg/simulation  the engine: World, entities, components, events, runtime
examples/       example models (CounterWorld first)
```

## Quick start

Requires Go 1.26+ (sandboxed toolchain at `~/opt/go` in this workspace).

```sh
export PATH=$HOME/opt/go/bin:$PATH
make test    # unit tests
make race    # race detector
make vet     # go vet
make fmt     # gofmt check
```

## Defining a model

A model implements `simulation.Model` plus one of `simulation.TickModel`
(discrete-time) or `simulation.EventModel` (discrete-event):

```go
type CounterWorld struct{ N int }

func (m *CounterWorld) Metadata() model.Metadata {
    return model.Metadata{ID: "counter", Name: "CounterWorld", Version: "1.0.0", Mode: model.ModeTick}
}
func (m *CounterWorld) Initialize(ctx context.Context, w *simulation.World) error { /* ... */ }
func (m *CounterWorld) Step(ctx context.Context, w *simulation.World) error       { /* ... */ }
```

See `examples/counter/` for a complete, runnable model.

## Determinism

A simulation is reproducible given the same model, seed, configuration, and
execution parameters. Randomness flows from a single master seed into named,
order-independent streams; there is no global RNG. Event ordering is a total
order on `(time, priority, sequence, id)`. See the determinism tests in
`examples/counter/counter_test.go`.

## Systems & parallel execution

A tick-mode model may decompose each tick into `System`s (via
`simulation.SystemModel`). Each system declares the components it reads and
writes; the scheduler orders systems by dependency (conflicting systems
serialize) and runs independent work in parallel across a bounded worker pool,
partitioning entities into contiguous, disjoint shards. Parallel execution is
deterministic: results are identical regardless of worker count (verified by
`TestCounterWorldParallelDeterministic`). Within a parallel shard, systems use
`Column.GetShard`/`SetShard` (lock-free, safe because shards own disjoint
entities).

Run the benchmark (results depend on hardware; a 2-core machine gives ~1.5x
parallel speedup on `CounterWorld`, which is memory-bound):

```sh
go test -run '^$' -bench BenchmarkCounterTick -benchmem ./examples/counter
```

## Snapshots & restore

A snapshot is a versioned, self-validating capture of a simulation's state:
provenance (simulation/model IDs, version, seed, mode), clock, entity
allocation state, all component columns (JSON-encoded), and the event queue,
with a SHA-256 checksum for integrity. `World.Snapshot()` captures state;
`World.Restore()` restores it (validating schema, engine version, checksum,
and model/seed match). Restore is deterministic: snapshotting mid-run and
continuing — in place or into a fresh simulation — reproduces the uninterrupted
run (see `examples/counter/snapshot_test.go`).

## Verification

```sh
go test ./...
go test -race ./...
go vet ./...
gofmt -l .
```
