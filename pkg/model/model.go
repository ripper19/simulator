// Package model defines the metadata and mode types shared between the
// simulation runtime and user-defined models. It is deliberately a leaf package
// with no dependency on the simulation engine so that models can describe
// themselves without coupling the engine to any domain.
package model

// Mode describes how a simulation advances time.
type Mode uint8

const (
	// ModeTick advances in discrete, fixed ticks (discrete-time simulation).
	ModeTick Mode = iota
	// ModeEvent advances by processing scheduled events (discrete-event simulation).
	ModeEvent
)

func (m Mode) String() string {
	switch m {
	case ModeTick:
		return "tick"
	case ModeEvent:
		return "event"
	default:
		return "unknown"
	}
}

// Metadata describes a model. The engine uses this to record provenance of a
// simulation so that a run can be reproduced even after newer model versions
// are released.
type Metadata struct {
	// ID is a stable, unique identifier for the model family, e.g. "counter".
	ID string
	// Name is a human-readable name.
	Name string
	// Version is a semantic version. A simulation records the exact version it
	// was executed with.
	Version string
	// Description is a short, free-form summary.
	Description string
	// Mode declares the execution mode(s) this model supports.
	Mode Mode
}
