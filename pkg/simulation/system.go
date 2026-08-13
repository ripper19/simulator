package simulation

import "context"

// System is a named unit of work with a declared component contract. A model
// composed of systems lets the scheduler order them by dependency and execute
// independent systems — and independent shards of a single system — in
// parallel while preserving deterministic results.
type System interface {
	// Name is a unique system name within the model. It is used to identify
	// the system and to order it deterministically among peers.
	Name() string
	// Reads lists the component IDs this system reads. The scheduler uses this
	// to determine which entities the system operates on and to detect
	// conflicts.
	Reads() []ComponentID
	// Writes lists the component IDs this system writes.
	Writes() []ComponentID
	// Run executes the system over a single shard: a deterministic, disjoint
	// partition of the entities the system operates on. Different shards of the
	// same system may run concurrently. Run may use Column.GetShard/SetShard
	// (lock-free) because the scheduler guarantees disjoint entity access, and
	// must not add or remove components or entities.
	Run(ctx context.Context, w *World, shard []EntityID) error
}

// SystemModel is implemented by models that express each tick as an ordered
// list of Systems rather than a single Step. The scheduler executes the systems
// in declared order, honoring read/write conflicts, and parallelizes
// independent work.
type SystemModel interface {
	Model
	// Systems returns the systems to run each tick, in declared precedence
	// order.
	Systems() []System
}
