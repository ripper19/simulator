package counter

import (
	"context"
	"testing"

	"github.com/ripper19/simulator/pkg/model"
	"github.com/ripper19/simulator/pkg/simulation"
)

func newSim(t *testing.T, seed uint64, n int, ticks uint64) *simulation.Simulation {
	t.Helper()
	m := &CounterWorld{N: n}
	sim, err := simulation.New(context.Background(), simulation.Config{
		ID:       "counter-test",
		Seed:     seed,
		Mode:     model.ModeTick,
		MaxTicks: ticks,
	}, m)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	return sim
}

func TestCounterWorldDeterministic(t *testing.T) {
	const n = 2000
	const ticks = 100
	a := newSim(t, 12345, n, ticks)
	if err := a.Run(context.Background()); err != nil {
		t.Fatalf("run a: %v", err)
	}
	b := newSim(t, 12345, n, ticks)
	if err := b.Run(context.Background()); err != nil {
		t.Fatalf("run b: %v", err)
	}

	fa := a.World().Components
	fb := b.World().Components
	// Same registry/columns, compare fingerprints via the models.
	ma := &CounterWorld{N: n}
	mb := &CounterWorld{N: n}
	ma.valueCol = simulation.ColumnOf[Value](fa, fa.Register("counter.value"))
	mb.valueCol = simulation.ColumnOf[Value](fb, fb.Register("counter.value"))
	if ma.Fingerprint(a.World()) != mb.Fingerprint(b.World()) {
		t.Fatal("identical seed/configuration produced different results")
	}
}

func TestCounterWorldDifferentSeed(t *testing.T) {
	const n = 2000
	const ticks = 100
	a := newSim(t, 1, n, ticks)
	a.Run(context.Background())
	b := newSim(t, 2, n, ticks)
	b.Run(context.Background())

	ma := &CounterWorld{N: n}
	mb := &CounterWorld{N: n}
	ma.valueCol = simulation.ColumnOf[Value](a.World().Components, a.World().Components.Register("counter.value"))
	mb.valueCol = simulation.ColumnOf[Value](b.World().Components, b.World().Components.Register("counter.value"))
	if ma.Fingerprint(a.World()) == mb.Fingerprint(b.World()) {
		t.Fatal("different seeds should produce different results")
	}
}

func TestCounterWorldEntityCount(t *testing.T) {
	m := &CounterWorld{N: 1000}
	sim, err := simulation.New(context.Background(), simulation.Config{
		ID: "c", Seed: 9, Mode: model.ModeTick, MaxTicks: 10,
	}, m)
	if err != nil {
		t.Fatal(err)
	}
	if sim.World().Entities.Len() != 1000 {
		t.Fatalf("expected 1000 entities, got %d", sim.World().Entities.Len())
	}
	if m.valueCol.Len() != 1000 {
		t.Fatalf("expected 1000 value components, got %d", m.valueCol.Len())
	}
}
