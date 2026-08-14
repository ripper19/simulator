package simulation

import (
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
}

func (w *World) bump() { w.revision.Add(1) }

// Revision returns the current structural revision.
func (w *World) Revision() uint64 { return w.revision.Load() }

// NewWorld creates an empty World rooted at the given seed.
func NewWorld(id string, seed uint64) *World {
	w := &World{
		ID:         id,
		Entities:   NewEntityManager(),
		Components: NewComponentStore(),
		Resources:  NewResourceRegistry(),
		Tags:       NewTagRegistry(),
		TagStore:   NewTagStore(),
		Events:     NewEventQueue(),
		Clock:      NewClock(),
		Random:     NewRandomStreams(seed),
	}
	w.Entities.bump = w.bump
	w.Components.bump = w.bump
	return w
}
