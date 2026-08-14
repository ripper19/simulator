package simulation

import (
	"context"
	"encoding/json"

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

// SnapshotModel is implemented by models that wish to include their own
// configuration in snapshots, so a restore can reconstruct identical behavior
// without the caller re-supplying configuration.
type SnapshotModel interface {
	// SnapshotConfig returns the model's configuration as a JSON-marshalable
	// value (exported fields).
	SnapshotConfig() any
	// RestoreConfig applies a previously snapshotted configuration, given as the
	// raw JSON produced by marshaling SnapshotConfig's result.
	RestoreConfig(raw json.RawMessage) error
}

// ConfigurableModel is implemented by models that accept a JSON configuration
// before Initialize (for example, from a simulation-creation request).
type ConfigurableModel interface {
	// Configure applies the model's configuration from raw JSON.
	Configure(raw json.RawMessage) error
}

// Metricer is implemented by models that expose measured outcomes (throughput,
// wait time, price, population, …) for reporting through the API's metrics
// endpoint. Keys and units are model-defined.
type Metricer interface {
	Metrics() map[string]float64
}
