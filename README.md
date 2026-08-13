# simulator

An open, general-purpose distributed simulation runtime written in Go for
executing deterministic, extensible simulations locally or across a scalable
worker cluster.

> **Status: Phase 2** — core engine (entities, SoA components, events, clock,
> deterministic randomness, and the simulation runtime), the discrete-event
> engine (priority event queue, immediate/delayed/prioritized scheduling), and
> the `CounterWorld` test model. Later phases add parallel execution, snapshots,
> persistence, REST API, distributed workers, and observability.

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

## Verification

```sh
go test ./...
go test -race ./...
go vet ./...
gofmt -l .
```
