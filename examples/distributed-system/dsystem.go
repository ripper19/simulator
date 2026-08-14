// Package dsystem is an example discrete-event model of a small distributed
// system: requests are routed to healthy servers, servers fail and recover.
// Availability depends on the interaction of request load and failure rate.
package dsystem

import (
	"context"
	"encoding/json"

	"github.com/ripper19/simulator/pkg/model"
	"github.com/ripper19/simulator/pkg/rng"
	"github.com/ripper19/simulator/pkg/simulation"
)

// SrvState is a server's health and current load.
type SrvState struct {
	Healthy bool
	Load    int
}

// Config is the JSON-configurable distributed-system scenario.
type Config struct {
	Servers         int     `json:"servers"`
	ArrivalInterval float64 `json:"arrival_interval"`
	ProcessingTime  float64 `json:"processing_time"`
	FailureInterval float64 `json:"failure_interval"`
	RecoveryTime    float64 `json:"recovery_time"`
	MaxTime         float64 `json:"max_time"`
}

func (c Config) withDefaults() Config {
	if c.Servers <= 0 {
		c.Servers = 4
	}
	if c.ArrivalInterval <= 0 {
		c.ArrivalInterval = 1
	}
	if c.ProcessingTime <= 0 {
		c.ProcessingTime = 2
	}
	if c.FailureInterval <= 0 {
		c.FailureInterval = 10
	}
	if c.RecoveryTime <= 0 {
		c.RecoveryTime = 5
	}
	if c.MaxTime <= 0 {
		c.MaxTime = 100
	}
	return c
}

// DistributedSystem routes requests across servers that fail and recover.
type DistributedSystem struct {
	cfg Config

	srvID  simulation.ComponentID
	srvCol *simulation.Column[SrvState]

	rng       rng.RNG
	rr        int
	processed int
	dropped   int
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

// Configure applies the scenario configuration.
func (m *DistributedSystem) Configure(raw json.RawMessage) error {
	var c Config
	if len(raw) == 0 {
		c = Config{}
	} else if err := json.Unmarshal(raw, &c); err != nil {
		return err
	}
	m.cfg = c.withDefaults()
	return nil
}

// Initialize creates servers and seeds the first request and failure.
func (m *DistributedSystem) Initialize(ctx context.Context, w *simulation.World) error {
	if m.cfg.Servers == 0 {
		m.cfg = m.cfg.withDefaults()
	}
	m.srvID, m.srvCol = simulation.RegisterComponent[SrvState](w.Components, "ds.state")
	m.rng = w.Random.StreamU64(1)
	for i := 0; i < m.cfg.Servers; i++ {
		m.srvCol.Set(w.Entities.Create(), SrvState{Healthy: true})
	}
	w.ScheduleNow("request", nil)
	w.ScheduleIn(m.cfg.FailureInterval, "fail", nil)
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
				Type: "done", Time: w.Clock.Time() + m.cfg.ProcessingTime, Source: srv,
			})
		} else {
			m.dropped++
		}
		if w.Clock.Time() < m.cfg.MaxTime {
			w.ScheduleIn(m.cfg.ArrivalInterval, "request", nil)
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
				Type: "recover", Time: w.Clock.Time() + m.cfg.RecoveryTime, Source: srv,
			})
		}
		if w.Clock.Time() < m.cfg.MaxTime {
			w.ScheduleIn(m.cfg.FailureInterval, "fail", nil)
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

// Metrics reports the measured outcomes.
func (m *DistributedSystem) Metrics() map[string]float64 {
	total := m.processed + m.dropped
	availability := 1.0
	if total > 0 {
		availability = float64(m.processed) / float64(total)
	}
	return map[string]float64{
		"processed":    float64(m.processed),
		"dropped":      float64(m.dropped),
		"availability": availability,
	}
}
