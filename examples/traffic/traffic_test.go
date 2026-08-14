package traffic

import (
	"context"
	"testing"

	"github.com/ripper19/simulator/pkg/model"
	"github.com/ripper19/simulator/pkg/simulation"
)

func run(t *testing.T, vehicles, intersections, ticks int) *simulation.Simulation {
	t.Helper()
	m := &Traffic{Vehicles: vehicles, Intersections: intersections, TicksPerLight: 10}
	sim, err := simulation.New(context.Background(), simulation.Config{
		ID: "traffic", Seed: 42, Mode: model.ModeTick, MaxTicks: uint64(ticks),
	}, m)
	if err != nil {
		t.Fatal(err)
	}
	if err := sim.Run(context.Background()); err != nil {
		t.Fatal(err)
	}
	return sim
}

func TestTrafficRunsAndMoves(t *testing.T) {
	sim := run(t, 20, 5, 100)
	if sim.State() != simulation.StateCompleted {
		t.Fatalf("state = %s", sim.State())
	}
	if sim.World().Entities.Len() != 25 {
		t.Fatalf("entities = %d, want 25", sim.World().Entities.Len())
	}
}

func TestTrafficDeterministic(t *testing.T) {
	a := run(t, 100, 10, 200)
	b := run(t, 100, 10, 200)
	if a.World().Clock.Tick() != b.World().Clock.Tick() {
		t.Fatal("tick mismatch")
	}
	// Position columns must be identical.
	ca := simulation.ColumnOf[Position](a.World().Components, a.World().Components.Register("traffic.position"))
	cb := simulation.ColumnOf[Position](b.World().Components, b.World().Components.Register("traffic.position"))
	sa, sb := 0, 0
	ca.Each(func(_ simulation.EntityID, v Position) { sa += v.X })
	cb.Each(func(_ simulation.EntityID, v Position) { sb += v.X })
	if sa != sb {
		t.Fatalf("position sum differs: %d vs %d", sa, sb)
	}
}
