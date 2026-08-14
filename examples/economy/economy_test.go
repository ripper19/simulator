package economy

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/ripper19/simulator/pkg/model"
	"github.com/ripper19/simulator/pkg/simulation"
)

func run(t *testing.T, cfg Config, ticks uint64) *Economy {
	t.Helper()
	m := &Economy{}
	b, _ := json.Marshal(cfg)
	if err := m.Configure(b); err != nil {
		t.Fatal(err)
	}
	sim, err := simulation.New(context.Background(), simulation.Config{
		ID: "econ", Seed: 3, Mode: model.ModeTick, MaxTicks: ticks,
	}, m)
	if err != nil {
		t.Fatal(err)
	}
	if err := sim.Run(context.Background()); err != nil {
		t.Fatal(err)
	}
	return m
}

func TestSupplyShockRaisesPrice(t *testing.T) {
	base := run(t, Config{Agents: 100, Productivity: 10, Demand: 10}, 200)
	shocked := run(t, Config{Agents: 100, Productivity: 10, Demand: 10, SupplyShockTick: 100, ShockFactor: 0.5}, 200)
	if shocked.Metrics()["max_price"] <= base.Metrics()["max_price"] {
		t.Fatalf("supply shock should raise max price: base %.3f shocked %.3f",
			base.Metrics()["max_price"], shocked.Metrics()["max_price"])
	}
}

func TestPriceCapBoundsPrice(t *testing.T) {
	capped := run(t, Config{Agents: 100, Productivity: 10, Demand: 10, SupplyShockTick: 100, ShockFactor: 0.5, PriceCap: 1.05}, 200)
	if capped.Metrics()["max_price"] > 1.05 {
		t.Fatalf("price exceeded cap: %.3f", capped.Metrics()["max_price"])
	}
}

func TestEconomyDeterministic(t *testing.T) {
	a := run(t, Config{Agents: 100, Productivity: 10, Demand: 10, SupplyShockTick: 50}, 200)
	b := run(t, Config{Agents: 100, Productivity: 10, Demand: 10, SupplyShockTick: 50}, 200)
	ma, mb := a.Metrics(), b.Metrics()
	for k := range ma {
		if ma[k] != mb[k] {
			t.Fatalf("metric %s differs: %v vs %v", k, ma[k], mb[k])
		}
	}
}
