// Package traffic is the flagship example: a linear road of intersections
// whose traffic lights gate vehicle movement, so congestion emerges from the
// interaction of demand and signal policy. Two policies (fixed timing vs
// adaptive) are selectable by configuration and produce different measured
// outcomes (throughput, average wait, peak queue) for the same seed and demand.
package traffic

import (
	"context"
	"encoding/json"
	"math"
	"sort"

	"github.com/ripper19/simulator/pkg/model"
	"github.com/ripper19/simulator/pkg/simulation"
)

// Config is the JSON-configurable traffic scenario.
type Config struct {
	Vehicles      int    `json:"vehicles"`
	Intersections int    `json:"intersections"`
	Spacing       int    `json:"spacing"`
	Policy        string `json:"policy"` // "fixed" (default) or "adaptive"
	GreenTicks    int    `json:"green_ticks"`
	RedTicks      int    `json:"red_ticks"`
	Threshold     int    `json:"threshold"` // adaptive: green when queue >= threshold
}

func (c Config) withDefaults() Config {
	if c.Vehicles <= 0 {
		c.Vehicles = 40
	}
	if c.Intersections <= 0 {
		c.Intersections = 5
	}
	if c.Spacing <= 0 {
		c.Spacing = 3
	}
	if c.Policy == "" {
		c.Policy = "fixed"
	}
	if c.GreenTicks <= 0 {
		c.GreenTicks = 5
	}
	if c.RedTicks <= 0 {
		c.RedTicks = 5
	}
	if c.Threshold <= 0 {
		c.Threshold = 2
	}
	return c
}

// Vehicle is a car on the road.
type Vehicle struct {
	Position int
	Wait     int
}

// Light is a traffic light: position, current phase, and the queue it served.
type Light struct {
	Pos   int
	Green bool
	Phase int
	Queue int
}

// Traffic runs the scenario.
type Traffic struct {
	cfg Config

	vehID    simulation.ComponentID
	vehCol   *simulation.Column[Vehicle]
	lightID  simulation.ComponentID
	lightCol *simulation.Column[Light]
	lightAt  map[int]simulation.EntityID

	completed int
	totalWait int
	peakQueue int
}

// Metadata describes the traffic model.
func (m *Traffic) Metadata() model.Metadata {
	return model.Metadata{
		ID:          "traffic",
		Name:        "Traffic",
		Version:     "1.0.0",
		Description: "Traffic lights gate vehicle movement; congestion emerges from demand x signal policy.",
		Mode:        model.ModeTick,
	}
}

// Configure applies the scenario configuration.
func (m *Traffic) Configure(raw json.RawMessage) error {
	if len(raw) == 0 {
		m.cfg = Config{}.withDefaults()
		return nil
	}
	var c Config
	if err := json.Unmarshal(raw, &c); err != nil {
		return err
	}
	m.cfg = c.withDefaults()
	return nil
}

// Initialize builds the road, lights, and vehicles.
func (m *Traffic) Initialize(ctx context.Context, w *simulation.World) error {
	if m.cfg.Intersections == 0 {
		m.cfg = m.cfg.withDefaults()
	}
	m.vehID, m.vehCol = simulation.RegisterComponent[Vehicle](w.Components, "traffic.vehicle")
	m.lightID, m.lightCol = simulation.RegisterComponent[Light](w.Components, "traffic.light")
	m.lightAt = make(map[int]simulation.EntityID)

	for i := 0; i < m.cfg.Intersections; i++ {
		pos := i * m.cfg.Spacing
		e := w.Entities.Create()
		m.lightCol.Set(e, Light{Pos: pos, Green: true})
		m.lightAt[pos] = e
	}
	for i := 0; i < m.cfg.Vehicles; i++ {
		e := w.Entities.Create()
		m.vehCol.Set(e, Vehicle{Position: -i - 1})
	}
	return nil
}

// Systems returns the light system (parallel) then the movement system
// (serial). The scheduler orders lights before movement because both touch the
// light component.
func (m *Traffic) Systems() []simulation.System {
	return []simulation.System{&lightSystem{m: m}, &movementSystem{m: m}}
}

// Metrics reports the measured outcomes.
func (m *Traffic) Metrics() map[string]float64 {
	return map[string]float64{
		"completed":  float64(m.completed),
		"throughput": float64(m.completed),
		"avg_wait":   float64(m.totalWait) / math.Max(1, float64(m.completed)),
		"peak_queue": float64(m.peakQueue),
	}
}

// lightSystem updates each light per the selected policy.
type lightSystem struct{ m *Traffic }

func (s *lightSystem) Name() string                     { return "traffic.light" }
func (s *lightSystem) Reads() []simulation.ComponentID  { return []simulation.ComponentID{s.m.lightID} }
func (s *lightSystem) Writes() []simulation.ComponentID { return []simulation.ComponentID{s.m.lightID} }

func (s *lightSystem) Run(ctx context.Context, w *simulation.World, shard []simulation.EntityID) error {
	for _, l := range shard {
		lt, ok := s.m.lightCol.GetShard(l)
		if !ok {
			continue
		}
		if s.m.cfg.Policy == "adaptive" {
			lt.Green = lt.Queue >= s.m.cfg.Threshold
		} else {
			cycle := s.m.cfg.GreenTicks + s.m.cfg.RedTicks
			lt.Phase = (lt.Phase + 1) % cycle
			lt.Green = lt.Phase < s.m.cfg.GreenTicks
		}
		s.m.lightCol.SetShard(l, lt)
	}
	return nil
}

// movementSystem advances vehicles in order (car-following), so it is serial.
type movementSystem struct{ m *Traffic }

func (s *movementSystem) Name() string { return "traffic.movement" }
func (s *movementSystem) Reads() []simulation.ComponentID {
	return []simulation.ComponentID{s.m.vehID, s.m.lightID}
}
func (s *movementSystem) Writes() []simulation.ComponentID {
	return []simulation.ComponentID{s.m.vehID, s.m.lightID}
}
func (s *movementSystem) Serial() bool { return true }

func (s *movementSystem) Run(ctx context.Context, w *simulation.World, shard []simulation.EntityID) error {
	m := s.m

	// Snapshot vehicles sorted front-to-back.
	type item struct {
		id simulation.EntityID
		v  Vehicle
	}
	var vs []item
	m.vehCol.Each(func(id simulation.EntityID, v Vehicle) { vs = append(vs, item{id, v}) })
	sort.Slice(vs, func(i, j int) bool { return vs[i].v.Position > vs[j].v.Position })

	occ := make(map[int]bool, len(vs))
	for _, it := range vs {
		occ[it.v.Position] = true
	}
	for _, l := range m.lightAt {
		lt, _ := m.lightCol.Get(l)
		lt.Queue = 0
		m.lightCol.Set(l, lt)
	}

	roadEnd := m.cfg.Intersections * m.cfg.Spacing
	for _, it := range vs {
		p := it.v.Position
		target := p + 1
		if target >= roadEnd {
			m.completed++
			m.totalWait += it.v.Wait
			w.Entities.Destroy(it.id)
			m.vehCol.Remove(it.id)
			delete(occ, p)
			continue
		}
		blocked := false
		if l, ok := m.lightAt[target]; ok {
			lt, _ := m.lightCol.Get(l)
			if !lt.Green {
				blocked = true
				lt.Queue++
				m.lightCol.Set(l, lt)
			}
		}
		if occ[target] {
			blocked = true
		}
		if blocked {
			it.v.Wait++
			m.vehCol.Set(it.id, it.v)
		} else {
			delete(occ, p)
			occ[target] = true
			it.v.Position = target
			m.vehCol.Set(it.id, it.v)
		}
	}

	q := 0
	for _, l := range m.lightAt {
		if lt, ok := m.lightCol.Get(l); ok {
			q += lt.Queue
		}
	}
	if q > m.peakQueue {
		m.peakQueue = q
	}
	return nil
}
