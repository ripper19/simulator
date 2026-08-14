package ecosystem

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/ripper19/simulator/pkg/model"
	"github.com/ripper19/simulator/pkg/simulation"
)

func run(t *testing.T, cfg Config, ticks int) (*simulation.Simulation, *Ecosystem) {
	t.Helper()
	m := &Ecosystem{}
	b, _ := json.Marshal(cfg)
	if err := m.Configure(b); err != nil {
		t.Fatal(err)
	}
	sim, err := simulation.New(context.Background(), simulation.Config{
		ID: "eco", Seed: 7, Mode: model.ModeTick, MaxTicks: uint64(ticks),
	}, m)
	if err != nil {
		t.Fatal(err)
	}
	if err := sim.Run(context.Background()); err != nil {
		t.Fatal(err)
	}
	return sim, m
}

func TestEcosystemRuns(t *testing.T) {
	sim, m := run(t, Config{Plants: 40, Animals: 6, MaxAnimals: 15}, 200)
	if sim.State() != simulation.StateCompleted {
		t.Fatalf("state = %s", sim.State())
	}
	if m.Metrics()["animals"] == 0 && m.Metrics()["plants"] == 0 {
		t.Fatal("ecosystem went extinct unexpectedly")
	}
}

// Showcase property: populations are bounded by carrying capacity and remain
// above zero (no extinction) across a long run.
func TestEcosystemStable(t *testing.T) {
	_, m := run(t, Config{Plants: 40, Animals: 6, MaxAnimals: 15}, 500)
	mm := m.Metrics()
	if mm["animals"] <= 0 {
		t.Fatal("animals went extinct")
	}
	if mm["animals"] > 15 {
		t.Fatalf("animal population %.0f exceeded carrying capacity", mm["animals"])
	}
}

func TestEcosystemDeterministic(t *testing.T) {
	_, a := run(t, Config{Plants: 40, Animals: 6, MaxAnimals: 15}, 300)
	_, b := run(t, Config{Plants: 40, Animals: 6, MaxAnimals: 15}, 300)
	ma, mb := a.Metrics(), b.Metrics()
	for k := range ma {
		if ma[k] != mb[k] {
			t.Fatalf("metric %s differs: %v vs %v", k, ma[k], mb[k])
		}
	}
}
