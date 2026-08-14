# ADR-004: Entity/component architecture

## Status
Accepted

## Context
Entities must be generic and cheap (up to millions). Component storage must be
fast for iteration, O(1) for access, and typed. Options: struct-of-arrays (SoA),
array-of-structs (AoS), archetypes, per-entity maps.

## Decision
- Entities are bare `EntityID` (index + generation) managed by a sparse set
  (`dense` list + `sparse` index), giving O(1) create/alive/destroy and
  generation-checked stale-ID rejection.
- Each component type is a typed `Column[T]` (SoA): parallel `dense []EntityID`
  and `data []T` with a `sparse` index. Iteration is cache-friendly and values
  are unboxed.
- Tags are a growable bitset per entity (lazily allocated).
- Resources are type-keyed singletons for shared, non-entity state.

## Consequences
- Iteration throughput and memory locality beat AoS and map-based stores.
- Cost: one `sparse` entry per ever-created entity index per component type
  (~4 bytes/entity/component).
- Lock-free `GetShard`/`SetShard` exist for the parallel scheduler's disjoint
  shard guarantee; the safe locked accessors remain for general use.
