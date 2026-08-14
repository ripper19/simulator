package counter

import (
	"context"
	"testing"

	"github.com/ripper19/simulator/pkg/model"
	"github.com/ripper19/simulator/pkg/simulation"
)

func newSim(t *testing.T, seed uint64, n int, ticks uint64, workers int) (*simulation.Simulation, *CounterWorld) {
	t.Helper()
	m := &CounterWorld{N: n}
	sim, err := simulation.New(context.Background(), simulation.Config{
		ID:       "counter-test",
		Seed:     seed,
		Mode:     model.ModeTick,
		MaxTicks: ticks,
		Workers:  workers,
	}, m)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	return sim, m
}

func TestCounterWorldDeterministic(t *testing.T) {
	const n = 2000
	const ticks = 100
	a, ma := newSim(t, 12345, n, ticks, 1)
	if err := a.Run(context.Background()); err != nil {
		t.Fatalf("run a: %v", err)
	}
	b, mb := newSim(t, 12345, n, ticks, 1)
	if err := b.Run(context.Background()); err != nil {
		t.Fatalf("run b: %v", err)
	}
	if ma.Fingerprint(a.World()) != mb.Fingerprint(b.World()) {
		t.Fatal("identical seed/configuration produced different results")
	}
}

func TestCounterWorldParallelDeterministic(t *testing.T) {
	const n = 50_000
	const ticks = 200
	serial, ms := newSim(t, 999, n, ticks, 1)
	if err := serial.Run(context.Background()); err != nil {
		t.Fatalf("serial run: %v", err)
	}
	parallel, mp := newSim(t, 999, n, ticks, 8)
	if err := parallel.Run(context.Background()); err != nil {
		t.Fatalf("parallel run: %v", err)
	}
	if ms.Fingerprint(serial.World()) != mp.Fingerprint(parallel.World()) {
		t.Fatal("parallel execution produced different results than serial execution")
	}
}

func TestCounterWorldDifferentSeed(t *testing.T) {
	const n = 2000
	const ticks = 100
	a, ma := newSim(t, 1, n, ticks, 1)
	a.Run(context.Background())
	b, mb := newSim(t, 2, n, ticks, 1)
	b.Run(context.Background())
	if ma.Fingerprint(a.World()) == mb.Fingerprint(b.World()) {
		t.Fatal("different seeds should produce different results")
	}
}

func TestCounterWorldEntityCount(t *testing.T) {
	sim, m := newSim(t, 9, 1000, 10, 1)
	if sim.World().Entities.Len() != 1000 {
		t.Fatalf("expected 1000 entities, got %d", sim.World().Entities.Len())
	}
	if m.valueCol.Len() != 1000 {
		t.Fatalf("expected 1000 value components, got %d", m.valueCol.Len())
	}
}
