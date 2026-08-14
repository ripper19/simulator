# simulator

An open, general-purpose distributed simulation runtime written in Go for
executing deterministic, extensible simulations locally or across a scalable
worker cluster.

> **Status: Phase 6** — core engine (entities, SoA components, events, clock,
> deterministic randomness, and the simulation runtime), the discrete-event
> engine, deterministic parallel execution, versioned snapshots with restore,
> PostgreSQL persistence (pgx + sqlc, migrations, model registry), and the REST
> API + `sim` CLI. Later phases add distributed workers, observability, and
> auth.

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

## Persistence

Durable state (model registry, simulations, snapshots) is stored in PostgreSQL
via `pgx` + `sqlc` (no ORM), with embedded SQL migrations. The model registry
supports multiple versions so a simulation pins the exact model version it ran
with.

```sh
export DATABASE_URL=postgres://simulator:simulator_dev_pw@127.0.0.1:5432/simulator?sslmode=disable
go run ./cmd/migrate           # apply migrations
go run ./cmd/migrate -down 1   # roll back one migration
```

Integration tests against a live PostgreSQL are run when `DATABASE_URL` is set
(each test is isolated in its own throwaway schema):

```sh
DATABASE_URL=... go test ./...
```

## API & CLI

The platform exposes a REST API (`cmd/api`, chi) for the model registry and
the full simulation lifecycle, plus a `sim` CLI (`cmd/cli`) that drives it over
HTTP. See `docs/api/openapi.yaml` for the endpoint reference.

```sh
export DATABASE_URL=postgres://simulator:simulator_dev_pw@127.0.0.1:5432/simulator?sslmode=disable
go run ./cmd/api                       # start the API on :8080
go run ./cmd/cli models                # list models
go run ./cmd/cli create --model counter --seed 42 --n 1000
go run ./cmd/cli start <id>
go run ./cmd/cli status <id>
go run ./cmd/cli stop <id>
go run ./cmd/cli snapshot <id>
```

## Verification

```sh
go test ./...
go test -race ./...
go vet ./...
gofmt -l .
```
