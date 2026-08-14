package queueing

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/ripper19/simulator/pkg/model"
	"github.com/ripper19/simulator/pkg/simulation"
)

func run(t *testing.T, cfg Config) (*simulation.Simulation, *Queueing) {
	t.Helper()
	m := &Queueing{}
	b, _ := json.Marshal(cfg)
	if err := m.Configure(b); err != nil {
		t.Fatal(err)
	}
	sim, err := simulation.New(context.Background(), simulation.Config{
		ID: "q", Seed: 123, Mode: model.ModeEvent,
	}, m)
	if err != nil {
		t.Fatal(err)
	}
	if err := sim.Run(context.Background()); err != nil {
		t.Fatal(err)
	}
	return sim, m
}

func TestQueueingServesAll(t *testing.T) {
	sim, m := run(t, Config{Servers: 3, Interarrival: 1, ServiceTime: 2, MaxTime: 100})
	if sim.State() != simulation.StateCompleted {
		t.Fatalf("state = %s", sim.State())
	}
	if m.Metrics()["served"] == 0 {
		t.Fatal("no customers served")
	}
	if m.served != m.arrived {
		t.Fatalf("served %d != arrived %d", m.served, m.arrived)
	}
}

// Showcase property: an under-provisioned queue (arrival > capacity) builds a
// backlog, while a well-provisioned one stays bounded.
func TestBacklogGrowsWhenUnderprovisioned(t *testing.T) {
	_, under := run(t, Config{Servers: 1, Interarrival: 1, ServiceTime: 2, MaxTime: 100})
	_, ok := run(t, Config{Servers: 4, Interarrival: 1, ServiceTime: 1, MaxTime: 100})
	if under.Metrics()["peak_backlog"] <= ok.Metrics()["peak_backlog"] {
		t.Fatalf("under-provisioned peak_backlog %.0f should exceed well-provisioned %.0f",
			under.Metrics()["peak_backlog"], ok.Metrics()["peak_backlog"])
	}
}

func TestQueueingDeterministic(t *testing.T) {
	_, a := run(t, Config{Servers: 3, Interarrival: 1, ServiceTime: 2, MaxTime: 100})
	_, b := run(t, Config{Servers: 3, Interarrival: 1, ServiceTime: 2, MaxTime: 100})
	ma, mb := a.Metrics(), b.Metrics()
	for k := range ma {
		if ma[k] != mb[k] {
			t.Fatalf("metric %s differs: %v vs %v", k, ma[k], mb[k])
		}
	}
}
