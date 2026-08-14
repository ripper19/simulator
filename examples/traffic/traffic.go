// Package traffic is the flagship example: a configurable road network whose
// traffic lights gate vehicle movement, so congestion emerges from the
// interaction of demand and signal policy. Light policies are pluggable via a
// LightAlgorithm interface (fixed timing vs adaptive, plus any user-supplied
// compiled-in algorithm), and outcomes (throughput, wait, queue) are measured.
package traffic

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
	"sort"
	"sync"

	"github.com/ripper19/simulator/pkg/model"
	"github.com/ripper19/simulator/pkg/rng"
	"github.com/ripper19/simulator/pkg/simulation"
)

// LightAlgorithm decides a light's green state each tick. A custom algorithm is
// plugged in via RegisterAlgorithm.
type LightAlgorithm interface {
	Name() string
	// Decide sets l.Green for this tick given the light's state and the queued
	// demand upstream of it.
	Decide(l *Light, queueLen int)
}

// Fixed cycles green then red for fixed durations.
type Fixed struct {
	GreenTicks int
	RedTicks   int
}

func (f Fixed) Name() string { return "fixed" }
func (f Fixed) Decide(l *Light, _ int) {
	cycle := f.GreenTicks + f.RedTicks
	l.Phase = (l.Phase + 1) % cycle
	l.Green = l.Phase < f.GreenTicks
}

// Adaptive turns green when the queue upstream reaches the threshold, and
// forces green after MaxRedTicks of red with waiting vehicles so stragglers
// (below the threshold) are not stranded.
type Adaptive struct {
	Threshold   int
	MaxRedTicks int
}

func (a Adaptive) Name() string { return "adaptive" }
func (a Adaptive) Decide(l *Light, q int) {
	if q >= a.Threshold {
		l.Green = true
		l.Phase = 0
		return
	}
	if l.Green {
		l.Green = false // demand dropped below threshold
		l.Phase = 0
		return
	}
	if q > 0 {
		l.Phase++
		if l.Phase >= a.MaxRedTicks {
			l.Green = true
			l.Phase = 0
		}
	}
}

var algorithmRegistry = struct {
	mu   sync.RWMutex
	algs map[string]LightAlgorithm
}{algs: map[string]LightAlgorithm{}}

// RegisterAlgorithm registers a custom compiled-in light algorithm.
func RegisterAlgorithm(alg LightAlgorithm) {
	algorithmRegistry.mu.Lock()
	algorithmRegistry.algs[alg.Name()] = alg
	algorithmRegistry.mu.Unlock()
}

// Node is an intersection in the network.
type Node struct {
	ID string `json:"id"`
}

// Edge is a road between two nodes.
type Edge struct {
	From   string `json:"from"`
	To     string `json:"to"`
	Length int    `json:"length"`
}

// Map is a user-supplied network topology and the route vehicles follow.
type Map struct {
	Nodes []Node   `json:"nodes"`
	Edges []Edge   `json:"edges"`
	Route []string `json:"route"` // ordered node IDs vehicles travel (entry -> exit)
}

// Config is the JSON-configurable traffic scenario.
type Config struct {
	Map           Map     `json:"map"`
	Intersections int     `json:"intersections"` // linear default when Map is empty
	Spacing       int     `json:"spacing"`
	Vehicles      int     `json:"vehicles"`
	Algorithm     string  `json:"algorithm"` // "fixed" | "adaptive" | registered
	GreenTicks    int     `json:"green_ticks"`
	RedTicks      int     `json:"red_ticks"`
	Threshold     int     `json:"threshold"`
	MaxRedTicks   int     `json:"max_red_ticks"` // adaptive: max red before forcing green
	SeedJitter    float64 `json:"seed_jitter"`   // >0 varies initial gaps with the seed
}

func (c Config) withDefaults() Config {
	if c.Vehicles <= 0 {
		c.Vehicles = 40
	}
	if c.Algorithm == "" {
		c.Algorithm = "fixed"
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
	if c.MaxRedTicks <= 0 {
		c.MaxRedTicks = 10
	}
	return c
}

// Light is a traffic light: green state, fixed-cycle phase, and the queue it
// served last tick (fed back into adaptive policies).
type Light struct {
	Green bool
	Phase int
	Queue int
}

// Vehicle travels a route edge-by-edge.
type Vehicle struct {
	Edge int // index into route; current edge is route[Edge] -> route[Edge+1]
	Pos  int // cells traveled on the edge; -ve = before entry, len = at the node
	Wait int
}

// Traffic runs the scenario.
type Traffic struct {
	cfg       Config
	algorithm LightAlgorithm

	vehID    simulation.ComponentID
	vehCol   *simulation.Column[Vehicle]
	lightID  simulation.ComponentID
	lightCol *simulation.Column[Light]

	route     []string
	edgeLen   map[int]int
	nodeLight map[string]simulation.EntityID

	rng       rng.RNG
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
		Description: "Traffic lights gate movement on a road network; congestion emerges from demand x policy.",
		Mode:        model.ModeTick,
	}
}

// Configure applies the scenario configuration.
func (m *Traffic) Configure(raw json.RawMessage) error {
	var c Config
	if len(raw) == 0 {
		c = Config{}
	} else if err := json.Unmarshal(raw, &c); err != nil {
		return err
	}
	m.cfg = c.withDefaults()
	return nil
}

// Initialize builds the network, lights, and vehicles.
func (m *Traffic) Initialize(ctx context.Context, w *simulation.World) error {
	if m.cfg.Vehicles == 0 {
		m.cfg = m.cfg.withDefaults()
	}
	m.vehID, m.vehCol = simulation.RegisterComponent[Vehicle](w.Components, "traffic.vehicle")
	m.lightID, m.lightCol = simulation.RegisterComponent[Light](w.Components, "traffic.light")
	m.nodeLight = make(map[string]simulation.EntityID)
	m.rng = w.Random.StreamU64(1)

	m.buildNetwork()
	m.resolveAlgorithm()

	for _, n := range m.route {
		m.nodeLight[n] = w.Entities.Create()
		m.lightCol.Set(m.nodeLight[n], Light{Green: true})
	}
	for i := 0; i < m.cfg.Vehicles; i++ {
		jitter := 0
		if m.cfg.SeedJitter > 0 {
			jitter = int(m.rng.Float64() * m.cfg.SeedJitter * float64(m.edgeLen[0]))
		}
		m.vehCol.Set(w.Entities.Create(), Vehicle{Edge: 0, Pos: -i - 1 - jitter})
	}
	return nil
}

// buildNetwork resolves the route and per-edge lengths from the config map, or
// builds a linear road when no map is supplied.
func (m *Traffic) buildNetwork() {
	m.edgeLen = make(map[int]int)
	if len(m.cfg.Map.Nodes) > 0 && len(m.cfg.Map.Route) > 0 {
		lengthByEdge := map[[2]string]int{}
		for _, e := range m.cfg.Map.Edges {
			lengthByEdge[[2]string{e.From, e.To}] = e.Length
		}
		m.route = append([]string(nil), m.cfg.Map.Route...)
		for i := 0; i < len(m.route)-1; i++ {
			if l, ok := lengthByEdge[[2]string{m.route[i], m.route[i+1]}]; ok {
				m.edgeLen[i] = l
			} else {
				m.edgeLen[i] = m.cfg.Spacing
			}
		}
		return
	}
	n := m.cfg.Intersections
	if n <= 1 {
		n = 5
	}
	for i := 0; i < n; i++ {
		m.route = append(m.route, fmt.Sprintf("n%d", i))
	}
	for i := 0; i < len(m.route)-1; i++ {
		m.edgeLen[i] = m.cfg.Spacing
	}
}

// resolveAlgorithm picks the configured light policy.
func (m *Traffic) resolveAlgorithm() {
	algorithmRegistry.mu.RLock()
	if alg, ok := algorithmRegistry.algs[m.cfg.Algorithm]; ok {
		m.algorithm = alg
		algorithmRegistry.mu.RUnlock()
		return
	}
	algorithmRegistry.mu.RUnlock()
	switch m.cfg.Algorithm {
	case "adaptive":
		m.algorithm = Adaptive{Threshold: m.cfg.Threshold, MaxRedTicks: m.cfg.MaxRedTicks}
	default:
		m.algorithm = Fixed{GreenTicks: m.cfg.GreenTicks, RedTicks: m.cfg.RedTicks}
	}
}

// Systems returns the light system (parallel) then the movement system
// (serial). Both touch the light component, so the scheduler orders lights
// before movement.
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

// lightSystem updates each light per the selected algorithm.
type lightSystem struct{ m *Traffic }

func (s *lightSystem) Name() string                     { return "traffic.light" }
func (s *lightSystem) Reads() []simulation.ComponentID  { return []simulation.ComponentID{s.m.lightID} }
func (s *lightSystem) Writes() []simulation.ComponentID { return []simulation.ComponentID{s.m.lightID} }

func (s *lightSystem) Run(ctx context.Context, w *simulation.World, shard []simulation.EntityID) error {
	for _, id := range shard {
		lt, ok := s.m.lightCol.GetShard(id)
		if !ok {
			continue
		}
		s.m.algorithm.Decide(&lt, lt.Queue)
		lt.Queue = 0
		s.m.lightCol.SetShard(id, lt)
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

	type item struct {
		id simulation.EntityID
		v  Vehicle
	}
	var vs []item
	m.vehCol.Each(func(id simulation.EntityID, v Vehicle) { vs = append(vs, item{id, v}) })
	sort.Slice(vs, func(i, j int) bool {
		if vs[i].v.Edge != vs[j].v.Edge {
			return vs[i].v.Edge > vs[j].v.Edge
		}
		return vs[i].v.Pos > vs[j].v.Pos
	})

	occ := make(map[int]map[int]bool)
	for _, it := range vs {
		if it.v.Pos >= 0 && it.v.Pos < m.edgeLen[it.v.Edge] {
			if occ[it.v.Edge] == nil {
				occ[it.v.Edge] = map[int]bool{}
			}
			occ[it.v.Edge][it.v.Pos] = true
		}
	}

	var completed, totalWait, queueSum int
	for _, it := range vs {
		e := it.v.Edge
		l := m.edgeLen[e]
		dest := m.route[e+1]
		p := it.v.Pos

		blocked := false
		finished := false
		switch {
		case p < 0:
			it.v.Pos = p + 1
		case p == l:
			if e == len(m.route)-2 {
				finished = true
			} else if !m.lightGreen(dest) {
				blocked = true
			} else if occ[e+1] != nil && occ[e+1][0] {
				blocked = true
			} else {
				it.v.Edge = e + 1
				it.v.Pos = 0
				if occ[e+1] == nil {
					occ[e+1] = map[int]bool{}
				}
				occ[e+1][0] = true
			}
		default: // 0 <= p < l
			target := p + 1
			if target == l {
				it.v.Pos = target
				delete(occ[e], p)
			} else if occ[e][target] {
				blocked = true
			} else {
				it.v.Pos = target
				delete(occ[e], p)
				occ[e][target] = true
			}
		}

		if finished {
			completed++
			totalWait += it.v.Wait
			w.Entities.Destroy(it.id)
			m.vehCol.Remove(it.id)
			continue
		}
		if blocked {
			it.v.Wait++
			queueSum++
			m.bumpQueue(dest)
		}
		m.vehCol.Set(it.id, it.v)
	}

	m.completed += completed
	m.totalWait += totalWait
	if queueSum > m.peakQueue {
		m.peakQueue = queueSum
	}
	return nil
}

func (m *Traffic) lightGreen(node string) bool {
	id, ok := m.nodeLight[node]
	if !ok {
		return true
	}
	lt, ok := m.lightCol.Get(id)
	return !ok || lt.Green
}

func (m *Traffic) bumpQueue(node string) {
	if id, ok := m.nodeLight[node]; ok {
		if lt, ok := m.lightCol.Get(id); ok {
			lt.Queue++
			m.lightCol.Set(id, lt)
		}
	}
}
