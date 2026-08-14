# ADR-007: Snapshot architecture

## Status
Accepted

## Context
Snapshots must capture enough state to restore execution deterministically,
survive serialization, be versioned, and detect corruption.

## Decision
- A `Snapshot` is a versioned JSON document: provenance (simulation/model IDs,
  version, seed, mode), clock, full entity allocation state, all component
  columns (JSON-encoded values), tags, resources, model config, and the event
  queue. A SHA-256 checksum over the canonical JSON detects corruption.
- Component values serialize via reflection/JSON (exported fields required).
- Event payload types are preserved via a type envelope when registered through
  `World.RegisterPayloadType`; unregistered payloads restore as opaque JSON.
- `SchemaVersion` and `EngineVersion` gate compatibility; `Validate` rejects
  mismatch or checksum failure.
- `Simulation.Snapshot` rejects a running simulation to avoid torn captures.

## Consequences
- Restore is deterministic (verified: restore+continue equals uninterrupted run).
- Trade-off: JSON is portable and debuggable but slower than a binary codec; a
  binary codec is a future optimization.
