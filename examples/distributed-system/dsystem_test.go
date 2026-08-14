package dsystem

import (
	"context"
	"testing"

	"github.com/ripper19/simulator/pkg/model"
	"github.com/ripper19/simulator/pkg/simulation"
)

func run(t *testing.T) (*simulation.Simulation, *DistributedSystem) {
	t.Helper()
	m := &DistributedSystem{
		Servers: 4, ArrivalInterval: 1, ProcessingTime: 2,
		FailureInterval: 10, RecoveryTime: 5, MaxTime: 100,
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
	sim, m := run(t)
	if sim.State() != simulation.StateCompleted {
		t.Fatalf("state = %s", sim.State())
	}
	if m.Processed() == 0 {
		t.Fatal("no requests processed")
	}
}

func TestDistributedSystemDeterministic(t *testing.T) {
	_, a := run(t)
	_, b := run(t)
	if a.Processed() != b.Processed() {
		t.Fatalf("non-deterministic: %d vs %d processed", a.Processed(), b.Processed())
	}
}
