package queueing

import (
	"context"
	"testing"

	"github.com/ripper19/simulator/pkg/model"
	"github.com/ripper19/simulator/pkg/simulation"
)

func run(t *testing.T) (*simulation.Simulation, *Queueing) {
	t.Helper()
	m := &Queueing{Servers: 3, Interarrival: 1.0, ServiceTime: 2.0, MaxTime: 100}
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
	sim, m := run(t)
	if sim.State() != simulation.StateCompleted {
		t.Fatalf("state = %s", sim.State())
	}
	if m.Served() == 0 {
		t.Fatal("no customers served")
	}
	// Queue must drain: every arrival eventually served.
	if m.Served() != m.Arrived() {
		t.Fatalf("served %d != arrived %d", m.Served(), m.Arrived())
	}
}

func TestQueueingDeterministic(t *testing.T) {
	_, a := run(t)
	_, b := run(t)
	if a.Served() != b.Served() || a.Arrived() != b.Arrived() {
		t.Fatalf("non-deterministic: %d/%d vs %d/%d", a.Arrived(), a.Served(), b.Arrived(), b.Served())
	}
}
