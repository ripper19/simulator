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
	m := &Traffic{}
	if err := m.Configure(mustJSON(cfg)); err != nil {
		t.Fatal(err)
	}
	sim, err := simulation.New(context.Background(), simulation.Config{
		ID: "traffic", Seed: 42, Mode: model.ModeTick, MaxTicks: ticks, Workers: 4,
	}, m)
	if err != nil {
		t.Fatal(err)
	}
	if err := sim.Run(context.Background()); err != nil {
		t.Fatal(err)
	}
	return sim, m
}

func mustJSON(v any) json.RawMessage {
	b, _ := json.Marshal(v)
	return b
}

func TestTrafficAllVehiclesComplete(t *testing.T) {
	sim, m := runCfg(t, Config{Vehicles: 40, Intersections: 5, Spacing: 3, Policy: "fixed", GreenTicks: 5, RedTicks: 5}, 1000)
	if sim.State() != simulation.StateCompleted {
		t.Fatalf("state = %s", sim.State())
	}
	if m.completed != 40 {
		t.Fatalf("completed %d, want 40", m.completed)
	}
	if m.Metrics()["avg_wait"] == 0 {
		t.Fatal("expected non-zero wait under red lights")
	}
}

// The showcase property: adaptive lights reduce average wait vs fixed timing
// for the same seed and demand.
func TestAdaptiveBeatsFixed(t *testing.T) {
	_, fixed := runCfg(t, Config{Vehicles: 60, Intersections: 5, Spacing: 3, Policy: "fixed", GreenTicks: 5, RedTicks: 5}, 2000)
	_, adaptive := runCfg(t, Config{Vehicles: 60, Intersections: 5, Spacing: 3, Policy: "adaptive", Threshold: 2}, 2000)
	fw := fixed.Metrics()["avg_wait"]
	aw := adaptive.Metrics()["avg_wait"]
	if aw >= fw {
		t.Fatalf("adaptive avg_wait %.2f should be < fixed %.2f", aw, fw)
	}
}

func TestTrafficDeterministic(t *testing.T) {
	_, a := runCfg(t, Config{Vehicles: 40, Intersections: 5, Spacing: 3, Policy: "adaptive", Threshold: 2}, 1000)
	_, b := runCfg(t, Config{Vehicles: 40, Intersections: 5, Spacing: 3, Policy: "adaptive", Threshold: 2}, 1000)
	ma, mb := a.Metrics(), b.Metrics()
	for k := range ma {
		if ma[k] != mb[k] {
			t.Fatalf("metric %s differs: %v vs %v", k, ma[k], mb[k])
		}
	}
}
