package simulation

import (
	"context"

	"github.com/ripper19/simulator/pkg/model"
)

// Model is the base contract every simulation model implements. It defines
// what the model is (metadata) and how it initializes a World. A model is
// domain-agnostic from the engine's perspective: the engine only calls these
// methods and never inspects domain state.
type Model interface {
	// Metadata describes the model identity and execution mode.
	Metadata() model.Metadata
	// Initialize builds the model's initial state into the World. It is called
	// exactly once, before the first step, and may create entities, register
	// components and tags, install resources, and schedule initial events.
	Initialize(ctx context.Context, w *World) error
}

// TickModel is implemented by discrete-time models that advance via fixed
// ticks. The runtime calls Step once per tick. Step may read and write the
// World but must not assume any particular tick cadence beyond the engine's
// contract.
type TickModel interface {
	Model
	Step(ctx context.Context, w *World) error
}

// EventModel is implemented by discrete-event models that advance by handling
// scheduled events. The runtime calls HandleEvent for each event popped from
// the queue; the model may schedule further events via w.Events.
type EventModel interface {
	Model
	HandleEvent(ctx context.Context, w *World, e Event) error
}
