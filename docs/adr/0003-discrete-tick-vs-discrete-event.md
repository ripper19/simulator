# ADR-003: Discrete tick vs discrete event

## Status
Accepted

## Context
Simulations advance either in fixed ticks (game loops, cellular automata) or by
jumping between scheduled events (queues, networks). Forcing one model breaks
the other.

## Decision
Support both, selected per model via `model.Mode`:
- `ModeTick`: the executor calls `Step` once per tick and advances the clock.
- `ModeEvent`: the executor pops the next event from a total-ordered priority
  queue, advances the clock to its time, and calls `HandleEvent`.

## Consequences
- `ModeEvent` ordering is a total order on `(time, priority, sequence, id)`,
  making scheduling order deterministic.
- `MaxTicks` bounds tick runs; `MaxTime` bounds event runs (checked before the
  next event fires, so an event at exactly MaxTime is not processed).
- A model authoring both modes would be a hybrid; not yet supported, deferred.
