package logistics

import (
	"context"
	"testing"

	"github.com/ripper19/simulator/pkg/model"
	"github.com/ripper19/simulator/pkg/simulation"
)

func run(t *testing.T) (*simulation.Simulation, *Logistics) {
	t.Helper()
	m := &Logistics{Warehouses: 2, Vehicles: 5, Orders: 20}
	sim, err := simulation.New(context.Background(), simulation.Config{
		ID: "log", Seed: 5, Mode: model.ModeTick, MaxTicks: 200,
	}, m)
	if err != nil {
		t.Fatal(err)
	}
	if err := sim.Run(context.Background()); err != nil {
		t.Fatal(err)
	}
	return sim, m
}

func TestLogisticsDeliversAll(t *testing.T) {
	sim, m := run(t)
	if sim.State() != simulation.StateCompleted {
		t.Fatalf("state = %s", sim.State())
	}
	if m.Delivered() != 20 {
		t.Fatalf("delivered %d, want 20", m.Delivered())
	}
}

func TestLogisticsDeterministic(t *testing.T) {
	_, a := run(t)
	_, b := run(t)
	if a.Delivered() != b.Delivered() {
		t.Fatalf("non-deterministic: %d vs %d delivered", a.Delivered(), b.Delivered())
	}
}
