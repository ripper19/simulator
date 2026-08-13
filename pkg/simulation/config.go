package simulation

import "github.com/ripper19/simulator/pkg/model"

// Config configures a single simulation run. It deliberately contains no
// domain-specific fields: everything model-specific belongs to the model's own
// configuration, which the model reads during Initialize (typically from a
// resource or its own fields).
type Config struct {
	// ID is the simulation identifier.
	ID string
	// Seed is the master random seed. The same seed with the same model and
	// configuration reproduces the same run.
	Seed uint64
	// Mode selects the execution strategy. It must match the model's declared
	// mode; if ModeTick is set the model must implement TickModel, and if
	// ModeEvent the model must implement EventModel.
	Mode model.Mode
	// MaxTicks, if > 0, stops a tick-mode run after this many ticks.
	MaxTicks uint64
	// MaxTime, if > 0, stops an event-mode run once simulation time reaches it.
	MaxTime float64
}
