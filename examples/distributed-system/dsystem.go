// Package dsystem is an example discrete-event model of a small distributed
// system: requests are routed to healthy servers, servers fail and recover.
package dsystem

import (
	"context"

	"github.com/ripper19/simulator/pkg/model"
	"github.com/ripper19/simulator/pkg/rng"
	"github.com/ripper19/simulator/pkg/simulation"
)

// SrvState is a server's health and current load.
type SrvState struct {
	Healthy bool
	Load    int
}

// DistributedSystem routes requests across servers that fail and recover.
type DistributedSystem struct {
	Servers         int
	ArrivalInterval float64
	ProcessingTime  float64
	FailureInterval float64
	RecoveryTime    float64
	MaxTime         float64

	srvID  simulation.ComponentID
	srvCol *simulation.Column[SrvState]

	rng       rng.RNG
	rr        int
	processed int
}

// Metadata describes the distributed-system model.
func (m *DistributedSystem) Metadata() model.Metadata {
	return model.Metadata{
		ID:          "distributed-system",
		Name:        "DistributedSystem",
		Version:     "1.0.0",
		Description: "Requests route across servers that fail and recover.",
		Mode:        model.ModeEvent,
	}
}

// Initialize creates servers and seeds the first request and failure.
func (m *DistributedSystem) Initialize(ctx context.Context, w *simulation.World) error {
	m.srvID, m.srvCol = simulation.RegisterComponent[SrvState](w.Components, "ds.state")
	m.rng = w.Random.StreamU64(1)
	for i := 0; i < m.Servers; i++ {
		m.srvCol.Set(w.Entities.Create(), SrvState{Healthy: true})
	}
	w.ScheduleNow("request", nil)
	w.ScheduleIn(m.FailureInterval, "fail", nil)
	return nil
}

// HandleEvent processes requests, completions, failures, and recoveries.
func (m *DistributedSystem) HandleEvent(ctx context.Context, w *simulation.World, e simulation.Event) error {
	switch e.Type {
	case "request":
		if srv, ok := m.pick(w); ok {
			st, _ := m.srvCol.Get(srv)
			st.Load++
			m.srvCol.Set(srv, st)
			w.Events.Push(simulation.Event{
				Type: "done", Time: w.Clock.Time() + m.ProcessingTime, Source: srv,
			})
		}
		if w.Clock.Time() < m.MaxTime {
			w.ScheduleIn(m.ArrivalInterval, "request", nil)
		}
	case "done":
		if st, ok := m.srvCol.Get(e.Source); ok {
			st.Load--
			m.srvCol.Set(e.Source, st)
			m.processed++
		}
	case "fail":
		if srv, ok := m.pick(w); ok {
			st, _ := m.srvCol.Get(srv)
			st.Healthy = false
			m.srvCol.Set(srv, st)
			w.Events.Push(simulation.Event{
				Type: "recover", Time: w.Clock.Time() + m.RecoveryTime, Source: srv,
			})
		}
		if w.Clock.Time() < m.MaxTime {
			w.ScheduleIn(m.FailureInterval, "fail", nil)
		}
	case "recover":
		if st, ok := m.srvCol.Get(e.Source); ok {
			st.Healthy = true
			m.srvCol.Set(e.Source, st)
		}
	}
	return nil
}

// pick returns the next healthy server in round-robin order.
func (m *DistributedSystem) pick(w *simulation.World) (simulation.EntityID, bool) {
	ids := w.Components.Entities(m.srvID)
	for range ids {
		m.rr = (m.rr + 1) % len(ids)
		id := ids[m.rr]
		if st, ok := m.srvCol.Get(id); ok && st.Healthy {
			return id, true
		}
	}
	return 0, false
}

// Processed returns the number of completed requests.
func (m *DistributedSystem) Processed() int { return m.processed }
