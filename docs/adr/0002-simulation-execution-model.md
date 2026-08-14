# ADR-002: Simulation execution model

## Status
Accepted

## Context
The engine must support both discrete-time (tick) and discrete-event execution
without coupling the runtime to a domain.

## Decision
The core abstraction is a `World` (state) driven by an `executor` chosen from
the model's declared mode. A model implements `Model` plus one of `TickModel`
(Step per tick) or `EventModel` (HandleEvent per event). A `SystemModel`
optional interface expresses a tick as dependency-ordered `System`s for
parallel execution. The runtime loop is identical across modes; only the
executor differs.

## Consequences
- A single `run` loop handles start/pause/resume/stop/step for both modes.
- New execution strategies can be added behind the `executor` seam.
- Models choose their mode; the runtime never guesses.
