package traffic

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/ripper19/simulator/pkg/model"
	"github.com/ripper19/simulator/pkg/simulation"
)

func runCfg(t *testing.T, cfg Config, ticks uint64) (*simulation.Simulation, *Traffic) {
	t.Helper()
	return runSeed(t, 42, cfg, ticks)
}

func runSeed(t *testing.T, seed uint64, cfg Config, ticks uint64) (*simulation.Simulation, *Traffic) {
	t.Helper()
	m := &Traffic{}
	b, _ := json.Marshal(cfg)
	if err := m.Configure(b); err != nil {
		t.Fatal(err)
	}
	sim, err := simulation.New(context.Background(), simulation.Config{
		ID: "traffic", Seed: seed, Mode: model.ModeTick, MaxTicks: ticks, Workers: 4,
	}, m)
	if err != nil {
		t.Fatal(err)
	}
	if err := sim.Run(context.Background()); err != nil {
		t.Fatal(err)
	}
	return sim, m
}

func TestTrafficAllVehiclesComplete(t *testing.T) {
	sim, m := runCfg(t, Config{Vehicles: 40, Intersections: 5, Spacing: 3, Algorithm: "fixed", GreenTicks: 5, RedTicks: 5}, 1000)
	if sim.State() != simulation.StateCompleted {
		t.Fatalf("state = %s", sim.State())
	}
	if m.Metrics()["throughput"] != 40 {
		t.Fatalf("throughput %v, want 40", m.Metrics()["throughput"])
	}
	if m.Metrics()["avg_wait"] == 0 {
		t.Fatal("expected non-zero wait under red lights")
	}
}

// Showcase property: adaptive lights reduce average wait vs fixed timing for the
// same seed and demand. Both policies must move traffic (throughput > 0), so a
// deadlock cannot pass this test vacuously.
func TestAdaptiveBeatsFixed(t *testing.T) {
	_, fixed := runCfg(t, Config{Vehicles: 60, Intersections: 5, Spacing: 3, Algorithm: "fixed", GreenTicks: 5, RedTicks: 5}, 2000)
	_, adaptive := runCfg(t, Config{Vehicles: 60, Intersections: 5, Spacing: 3, Algorithm: "adaptive", Threshold: 2}, 2000)

	if fixed.Metrics()["throughput"] != 60 {
		t.Fatalf("fixed policy deadlocked: throughput %v", fixed.Metrics()["throughput"])
	}
	if adaptive.Metrics()["throughput"] != 60 {
		t.Fatalf("adaptive policy deadlocked: throughput %v", adaptive.Metrics()["throughput"])
	}
	if adaptive.Metrics()["avg_wait"] >= fixed.Metrics()["avg_wait"] {
		t.Fatalf("adaptive avg_wait %.2f should be < fixed %.2f",
			adaptive.Metrics()["avg_wait"], fixed.Metrics()["avg_wait"])
	}
}

func TestTrafficDeterministic(t *testing.T) {
	_, a := runCfg(t, Config{Vehicles: 40, Intersections: 5, Spacing: 3, Algorithm: "adaptive", Threshold: 2}, 1000)
	_, b := runCfg(t, Config{Vehicles: 40, Intersections: 5, Spacing: 3, Algorithm: "adaptive", Threshold: 2}, 1000)
	ma, mb := a.Metrics(), b.Metrics()
	for k := range ma {
		if ma[k] != mb[k] {
			t.Fatalf("metric %s differs: %v vs %v", k, ma[k], mb[k])
		}
	}
}

// A user-supplied road network: three nodes, two edges, a fixed route.
func TestCustomNetwork(t *testing.T) {
	cfg := Config{
		Map: Map{
			Nodes: []Node{{ID: "A"}, {ID: "B"}, {ID: "C"}},
			Edges: []Edge{{From: "A", To: "B", Length: 4}, {From: "B", To: "C", Length: 4}},
			Route: []string{"A", "B", "C"},
		},
		Vehicles:   30,
		Algorithm:  "fixed",
		GreenTicks: 4,
		RedTicks:   4,
	}
	sim, m := runCfg(t, cfg, 1000)
	if sim.State() != simulation.StateCompleted {
		t.Fatalf("state = %s", sim.State())
	}
	if m.Metrics()["throughput"] != 30 {
		t.Fatalf("throughput %v, want 30", m.Metrics()["throughput"])
	}
}

// A custom compiled-in light algorithm can be plugged in.
func TestPluggableAlgorithm(t *testing.T) {
	RegisterAlgorithm(alwaysGreen{})
	defer delete(algorithmRegistry.algs, "always-green")

	_, m := runCfg(t, Config{Vehicles: 20, Intersections: 3, Spacing: 2, Algorithm: "always-green"}, 500)
	if m.Metrics()["throughput"] != 20 {
		t.Fatalf("throughput %v, want 20", m.Metrics()["throughput"])
	}
	// always-green avoids red-light stops, so it must beat fixed timing on wait.
	_, fixed := runCfg(t, Config{Vehicles: 20, Intersections: 3, Spacing: 2, Algorithm: "fixed", GreenTicks: 3, RedTicks: 3}, 500)
	if m.Metrics()["avg_wait"] >= fixed.Metrics()["avg_wait"] {
		t.Fatalf("always-green avg_wait %v should be < fixed %v",
			m.Metrics()["avg_wait"], fixed.Metrics()["avg_wait"])
	}
}

// Showcase property: the seed meaningfully changes congestion (via per-vehicle
// speeds), so circumstances differ across seeds.
func TestSeedChangesOutcome(t *testing.T) {
	cfg := Config{Vehicles: 80, Intersections: 5, Spacing: 3, Algorithm: "fixed", GreenTicks: 5, RedTicks: 5}
	_, a := runSeed(t, 42, cfg, 2000)
	_, b := runSeed(t, 43, cfg, 2000)
	ma, mb := a.Metrics(), b.Metrics()
	if ma["peak_queue"] == mb["peak_queue"] && ma["avg_wait"] == mb["avg_wait"] {
		t.Fatalf("different seeds should change congestion: %v vs %v", ma, mb)
	}
}

type alwaysGreen struct{}

func (alwaysGreen) Name() string { return "always-green" }
func (alwaysGreen) Decide(l *Light, _ int) {
	l.Green = true
}
