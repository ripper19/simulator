package dsystem

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/ripper19/simulator/pkg/model"
	"github.com/ripper19/simulator/pkg/simulation"
)

func run(t *testing.T, cfg Config) (*simulation.Simulation, *DistributedSystem) {
	t.Helper()
	m := &DistributedSystem{}
	b, _ := json.Marshal(cfg)
	if err := m.Configure(b); err != nil {
		t.Fatal(err)
	}
	sim, err := simulation.New(context.Background(), simulation.Config{
		ID: "ds", Seed: 9, Mode: model.ModeEvent,
	}, m)
	if err != nil {
		t.Fatal(err)
	}
	if err := sim.Run(context.Background()); err != nil {
		t.Fatal(err)
	}
	return sim, m
}

func TestDistributedSystemProcesses(t *testing.T) {
	sim, m := run(t, Config{})
	if sim.State() != simulation.StateCompleted {
		t.Fatalf("state = %s", sim.State())
	}
	if m.Metrics()["processed"] == 0 {
		t.Fatal("no requests processed")
	}
}

// Showcase property: more frequent failures (or slower recovery) lowers
// availability. A single-server system makes the difference measurable.
func TestFailureRateLowersAvailability(t *testing.T) {
	_, healthy := run(t, Config{Servers: 1, FailureInterval: 100, RecoveryTime: 1, MaxTime: 200})
	_, flaky := run(t, Config{Servers: 1, FailureInterval: 10, RecoveryTime: 20, MaxTime: 200})
	if flaky.Metrics()["availability"] >= healthy.Metrics()["availability"] {
		t.Fatalf("flaky availability %.3f should be < healthy %.3f",
			flaky.Metrics()["availability"], healthy.Metrics()["availability"])
	}
}

func TestDistributedSystemDeterministic(t *testing.T) {
	_, a := run(t, Config{FailureInterval: 10, RecoveryTime: 5})
	_, b := run(t, Config{FailureInterval: 10, RecoveryTime: 5})
	ma, mb := a.Metrics(), b.Metrics()
	for k := range ma {
		if ma[k] != mb[k] {
			t.Fatalf("metric %s differs: %v vs %v", k, ma[k], mb[k])
		}
	}
}
