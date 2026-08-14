package simulation

import (
	"reflect"
	"sync"
	"sync/atomic"

	"github.com/ripper19/simulator/pkg/model"
)

// Metadata records the provenance of a running simulation, which is required
// to reproduce a run.
type Metadata struct {
	SimulationID string
	ModelID      string
	ModelVersion string
	Seed         uint64
	Mode         model.Mode
}

// World is the domain-agnostic simulation state container. It exposes the
// primitive building blocks (entities, components, resources, tags, events, a
// clock, and random streams) from which any model constructs its own domain
// concepts. The World knows nothing about what an entity "is"; that meaning is
// supplied by the model.
type World struct {
	ID         string
	Entities   *EntityManager
	Components *ComponentStore
	Resources  *ResourceRegistry
	Tags       *TagRegistry
	TagStore   *TagStore
	Events     *EventQueue
	Clock      *Clock
	Random     *RandomStreams
	Meta       Metadata

	// revision increments on every structural change (entity create/destroy,
	// component add/remove). It lets the scheduler cache derived data (such as a
	// system's entity set) and invalidate only when structure changes.
	revision atomic.Uint64

	// payloadTypes maps a registered payload Go type name to its reflect.Type,
	// so event payloads can round-trip through a snapshot preserving their type.
	payloadMu    sync.RWMutex
	payloadTypes map[string]reflect.Type
}

func (w *World) bump() { w.revision.Add(1) }

// Revision returns the current structural revision.
func (w *World) Revision() uint64 { return w.revision.Load() }

// RegisterPayloadType registers a payload type so its values survive a
// snapshot/restore with their Go type intact. Pass an example value (e.g.
// MyPayload{}). Payload types must be JSON-marshalable (exported fields).
// Unregistered payload types are still snapshotted, but restore as opaque JSON
// (json.RawMessage).
func (w *World) RegisterPayloadType(example any) {
	t := reflect.TypeOf(example)
	w.payloadMu.Lock()
	w.payloadTypes[t.String()] = t
	w.payloadMu.Unlock()
}

func (w *World) payloadTypeSet() map[string]reflect.Type {
	w.payloadMu.RLock()
	defer w.payloadMu.RUnlock()
	out := make(map[string]reflect.Type, len(w.payloadTypes))
	for k, v := range w.payloadTypes {
		out[k] = v
	}
	return out
}

// NewWorld creates an empty World rooted at the given seed.
func NewWorld(id string, seed uint64) *World {
	w := &World{
		ID:           id,
		Entities:     NewEntityManager(),
		Components:   NewComponentStore(),
		Resources:    NewResourceRegistry(),
		Tags:         NewTagRegistry(),
		TagStore:     NewTagStore(),
		Events:       NewEventQueue(),
		Clock:        NewClock(),
		Random:       NewRandomStreams(seed),
		payloadTypes: make(map[string]reflect.Type),
	}
	w.Entities.bump = w.bump
	w.Components.bump = w.bump
	return w
}
