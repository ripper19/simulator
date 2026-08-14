// Package queueing is an example discrete-event model: customers arrive,
// queue, and are served by a pool of servers.
package queueing

import (
	"context"

	"github.com/ripper19/simulator/pkg/model"
	"github.com/ripper19/simulator/pkg/rng"
	"github.com/ripper19/simulator/pkg/simulation"
)

// Busy marks whether a server is currently serving.
type Busy struct{ B bool }

// Queueing is an M/M/s-style queue simulated with discrete events.
type Queueing struct {
	Servers      int
	Interarrival float64 // mean interarrival time
	ServiceTime  float64 // mean service time
	MaxTime      float64 // stop scheduling arrivals past this time

	busyID      simulation.ComponentID
	busyCol     *simulation.Column[Busy]
	customerTag simulation.Tag

	rng     rng.RNG
	queue   []simulation.EntityID
	arrived int
	served  int
}

// Metadata describes the queueing model.
func (m *Queueing) Metadata() model.Metadata {
	return model.Metadata{
		ID:          "queueing",
		Name:        "Queueing",
		Version:     "1.0.0",
		Description: "Customers arrive, queue, and are served by servers.",
		Mode:        model.ModeEvent,
	}
}

// Initialize creates servers and schedules the first arrival.
func (m *Queueing) Initialize(ctx context.Context, w *simulation.World) error {
	m.busyID, m.busyCol = simulation.RegisterComponent[Busy](w.Components, "q.busy")
	m.customerTag = w.Tags.Register("customer")
	m.rng = w.Random.StreamU64(1)

	for i := 0; i < m.Servers; i++ {
		m.busyCol.Set(w.Entities.Create(), Busy{})
	}
	w.ScheduleNow("arrival", nil)
	return nil
}

// HandleEvent processes arrivals and service completions.
func (m *Queueing) HandleEvent(ctx context.Context, w *simulation.World, e simulation.Event) error {
	switch e.Type {
	case "arrival":
		m.arrived++
		c := w.Entities.Create()
		w.TagStore.Add(c, m.customerTag)
		m.queue = append(m.queue, c)
		m.dispatch(w)
		if e.Time < m.MaxTime {
			delay := m.Interarrival * (0.5 + m.rng.Float64())
			w.ScheduleIn(delay, "arrival", nil)
		}
	case "done":
		m.busyCol.Set(e.Source, Busy{})
		m.served++
		m.dispatch(w)
	}
	return nil
}

// dispatch assigns waiting customers to free servers.
func (m *Queueing) dispatch(w *simulation.World) {
	for len(m.queue) > 0 {
		var free simulation.EntityID
		found := false
		m.busyCol.Each(func(id simulation.EntityID, b Busy) {
			if !found && !b.B {
				free, found = id, true
			}
		})
		if !found {
			return
		}
		c := m.queue[0]
		m.queue = m.queue[1:]
		m.busyCol.Set(free, Busy{B: true})
		w.Entities.Destroy(c)
		w.TagStore.RemoveEntity(c)
		delay := m.ServiceTime * (0.5 + m.rng.Float64())
		w.Events.Push(simulation.Event{Type: "done", Time: w.Clock.Time() + delay, Source: free})
	}
}

// Served returns the number of completed services (for tests/metrics).
func (m *Queueing) Served() int { return m.served }

// Arrived returns the number of arrivals.
func (m *Queueing) Arrived() int { return m.arrived }
