// Package queueing is an example discrete-event model: customers arrive,
// queue, and are served by a pool of servers. Backlog grows or stays bounded
// depending on arrival rate vs service capacity.
package queueing

import (
	"context"
	"encoding/json"

	"github.com/ripper19/simulator/pkg/model"
	"github.com/ripper19/simulator/pkg/rng"
	"github.com/ripper19/simulator/pkg/simulation"
)

// Busy marks whether a server is currently serving.
type Busy struct{ B bool }

// Config is the JSON-configurable queueing scenario.
type Config struct {
	Servers      int     `json:"servers"`
	Interarrival float64 `json:"interarrival"` // mean interarrival time
	ServiceTime  float64 `json:"service_time"` // mean service time
	MaxTime      float64 `json:"max_time"`     // stop scheduling arrivals past this time
}

func (c Config) withDefaults() Config {
	if c.Servers <= 0 {
		c.Servers = 3
	}
	if c.Interarrival <= 0 {
		c.Interarrival = 1.0
	}
	if c.ServiceTime <= 0 {
		c.ServiceTime = 2.0
	}
	if c.MaxTime <= 0 {
		c.MaxTime = 100
	}
	return c
}

// Queueing is an M/M/s-style queue simulated with discrete events.
type Queueing struct {
	cfg Config

	busyID      simulation.ComponentID
	busyCol     *simulation.Column[Busy]
	customerTag simulation.Tag

	rng     rng.RNG
	queue   []simulation.EntityID
	arrived int
	served  int
	peak    int
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

// Configure applies the scenario configuration.
func (m *Queueing) Configure(raw json.RawMessage) error {
	var c Config
	if len(raw) == 0 {
		c = Config{}
	} else if err := json.Unmarshal(raw, &c); err != nil {
		return err
	}
	m.cfg = c.withDefaults()
	return nil
}

// Initialize creates servers and schedules the first arrival.
func (m *Queueing) Initialize(ctx context.Context, w *simulation.World) error {
	if m.cfg.Servers == 0 {
		m.cfg = m.cfg.withDefaults()
	}
	m.busyID, m.busyCol = simulation.RegisterComponent[Busy](w.Components, "q.busy")
	m.customerTag = w.Tags.Register("customer")
	m.rng = w.Random.StreamU64(1)

	for i := 0; i < m.cfg.Servers; i++ {
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
		if len(m.queue) > m.peak {
			m.peak = len(m.queue)
		}
		m.dispatch(w)
		if e.Time < m.cfg.MaxTime {
			delay := m.cfg.Interarrival * (0.5 + m.rng.Float64())
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
		delay := m.cfg.ServiceTime * (0.5 + m.rng.Float64())
		w.Events.Push(simulation.Event{Type: "done", Time: w.Clock.Time() + delay, Source: free})
	}
}

// Metrics reports the measured outcomes.
func (m *Queueing) Metrics() map[string]float64 {
	utilization := 0.0
	if m.arrived > 0 {
		utilization = float64(m.served) / float64(m.arrived)
	}
	return map[string]float64{
		"arrived":      float64(m.arrived),
		"served":       float64(m.served),
		"peak_backlog": float64(m.peak),
		"completion":   utilization,
	}
}
