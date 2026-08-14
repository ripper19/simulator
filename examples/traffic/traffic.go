// Package traffic is an example discrete-tick model demonstrating the engine's
// domain-agnosticism: vehicles and intersections with traffic lights.
package traffic

import (
	"context"

	"github.com/ripper19/simulator/pkg/model"
	"github.com/ripper19/simulator/pkg/simulation"
)

// Position is a vehicle's location along a single-lane road.
type Position struct{ X int }

// Speed is a vehicle's per-tick movement rate.
type Speed struct{ V int }

// Light is an intersection's traffic light; Green=true means vehicles may pass.
type Light struct{ Green bool }

// Traffic simulates vehicles moving through intersections whose lights toggle.
type Traffic struct {
	Vehicles      int
	Intersections int
	TicksPerLight int

	posID, speedID, lightID simulation.ComponentID
	posCol                  *simulation.Column[Position]
	speedCol                *simulation.Column[Speed]
	lightCol                *simulation.Column[Light]
}

// Metadata describes the traffic model.
func (m *Traffic) Metadata() model.Metadata {
	return model.Metadata{
		ID:          "traffic",
		Name:        "Traffic",
		Version:     "1.0.0",
		Description: "Vehicles move through intersections with toggling traffic lights.",
		Mode:        model.ModeTick,
	}
}

// Initialize creates vehicles and intersections.
func (m *Traffic) Initialize(ctx context.Context, w *simulation.World) error {
	m.posID, m.posCol = simulation.RegisterComponent[Position](w.Components, "traffic.position")
	m.speedID, m.speedCol = simulation.RegisterComponent[Speed](w.Components, "traffic.speed")
	m.lightID, m.lightCol = simulation.RegisterComponent[Light](w.Components, "traffic.light")

	stream := w.Random.StreamU64(1)
	for i := 0; i < m.Intersections; i++ {
		e := w.Entities.Create()
		m.lightCol.Set(e, Light{Green: i%2 == 0})
	}
	for i := 0; i < m.Vehicles; i++ {
		e := w.Entities.Create()
		m.posCol.Set(e, Position{})
		m.speedCol.Set(e, Speed{V: int(stream.Uint64n(5)) + 1})
	}
	return nil
}

// Systems returns the traffic systems.
func (m *Traffic) Systems() []simulation.System {
	return []simulation.System{&lightSystem{m: m}, &movementSystem{m: m}}
}

// lightSystem toggles each light every TicksPerLight ticks.
type lightSystem struct{ m *Traffic }

func (s *lightSystem) Name() string                     { return "traffic.light" }
func (s *lightSystem) Reads() []simulation.ComponentID  { return []simulation.ComponentID{s.m.lightID} }
func (s *lightSystem) Writes() []simulation.ComponentID { return []simulation.ComponentID{s.m.lightID} }
func (s *lightSystem) Run(ctx context.Context, w *simulation.World, shard []simulation.EntityID) error {
	if w.Clock.Tick()%uint64(s.m.TicksPerLight) != 0 {
		return nil
	}
	for _, e := range shard {
		l, _ := s.m.lightCol.GetShard(e)
		l.Green = !l.Green
		s.m.lightCol.SetShard(e, l)
	}
	return nil
}

// movementSystem advances each vehicle by its speed.
type movementSystem struct{ m *Traffic }

func (s *movementSystem) Name() string { return "traffic.movement" }
func (s *movementSystem) Reads() []simulation.ComponentID {
	return []simulation.ComponentID{s.m.posID, s.m.speedID}
}
func (s *movementSystem) Writes() []simulation.ComponentID {
	return []simulation.ComponentID{s.m.posID}
}
func (s *movementSystem) Run(ctx context.Context, w *simulation.World, shard []simulation.EntityID) error {
	for _, e := range shard {
		sp, ok := s.m.speedCol.GetShard(e)
		if !ok {
			continue
		}
		p, _ := s.m.posCol.GetShard(e)
		p.X += sp.V
		s.m.posCol.SetShard(e, p)
	}
	return nil
}
