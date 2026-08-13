package simulation

import "github.com/ripper19/simulator/pkg/model"

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
}

// NewWorld creates an empty World rooted at the given seed.
func NewWorld(id string, seed uint64) *World {
	return &World{
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
}
