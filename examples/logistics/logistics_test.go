package logistics

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/ripper19/simulator/pkg/model"
	"github.com/ripper19/simulator/pkg/simulation"
)

func run(t *testing.T, cfg Config) (*simulation.Simulation, *Logistics) {
	t.Helper()
	m := &Logistics{}
	b, _ := json.Marshal(cfg)
	if err := m.Configure(b); err != nil {
		t.Fatal(err)
	}
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
	sim, m := run(t, Config{Warehouses: 2, Vehicles: 5, Orders: 20})
	if sim.State() != simulation.StateCompleted {
		t.Fatalf("state = %s", sim.State())
	}
	if m.Delivered() != 20 {
		t.Fatalf("delivered %d, want 20", m.Delivered())
	}
}

func TestLogisticsDeterministic(t *testing.T) {
	_, a := run(t, Config{Warehouses: 2, Vehicles: 5, Orders: 20})
	_, b := run(t, Config{Warehouses: 2, Vehicles: 5, Orders: 20})
	ma, mb := a.Metrics(), b.Metrics()
	for k := range ma {
		if ma[k] != mb[k] {
			t.Fatalf("metric %s differs: %v vs %v", k, ma[k], mb[k])
		}
	}
}
