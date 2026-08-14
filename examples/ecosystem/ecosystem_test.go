package ecosystem

import (
	"context"
	"testing"

	"github.com/ripper19/simulator/pkg/model"
	"github.com/ripper19/simulator/pkg/simulation"
)

func run(t *testing.T, plants, animals, ticks int) *simulation.Simulation {
	t.Helper()
	m := &Ecosystem{Plants: plants, Animals: animals, MaxAnimals: 15}
	sim, err := simulation.New(context.Background(), simulation.Config{
		ID: "eco", Seed: 7, Mode: model.ModeTick, MaxTicks: uint64(ticks),
	}, m)
	if err != nil {
		t.Fatal(err)
	}
	if err := sim.Run(context.Background()); err != nil {
		t.Fatal(err)
	}
	return sim
}

func totalEnergy(w *simulation.World) int {
	col := simulation.ColumnOf[Energy](w.Components, w.Components.Register("eco.energy"))
	sum := 0
	col.Each(func(_ simulation.EntityID, v Energy) { sum += v.E })
	return sum
}

func TestEcosystemRuns(t *testing.T) {
	sim := run(t, 40, 6, 200)
	if sim.State() != simulation.StateCompleted {
		t.Fatalf("state = %s", sim.State())
	}
	if sim.World().Entities.Len() == 0 {
		t.Fatal("ecosystem went extinct unexpectedly")
	}
}

func TestEcosystemDeterministic(t *testing.T) {
	a := run(t, 40, 6, 300)
	b := run(t, 40, 6, 300)
	if a.World().Entities.Len() != b.World().Entities.Len() {
		t.Fatalf("population differs: %d vs %d", a.World().Entities.Len(), b.World().Entities.Len())
	}
	if totalEnergy(a.World()) != totalEnergy(b.World()) {
		t.Fatal("total energy differs across runs")
	}
}
