package simulation

import (
	"context"
	"testing"

	"github.com/ripper19/simulator/pkg/model"
)

// fnSystem is a minimal System for scheduler unit tests.
type fnSystem struct {
	name          string
	reads, writes []ComponentID
	run           func(ctx context.Context, w *World, shard []EntityID) error
}

func (s *fnSystem) Name() string          { return s.name }
func (s *fnSystem) Reads() []ComponentID  { return s.reads }
func (s *fnSystem) Writes() []ComponentID { return s.writes }
func (s *fnSystem) Run(ctx context.Context, w *World, shard []EntityID) error {
	if s.run == nil {
		return nil
	}
	return s.run(ctx, w, shard)
}

func TestDependencyLevelsConflictOrdering(t *testing.T) {
	c1, c2, c3, c4 := ComponentID(1), ComponentID(2), ComponentID(3), ComponentID(4)

	// A writes c1. B reads c1 and writes c2. C writes c2. D/E are independent.
	systems := []System{
		&fnSystem{name: "A", writes: []ComponentID{c1}},
		&fnSystem{name: "B", reads: []ComponentID{c1}, writes: []ComponentID{c2}},
		&fnSystem{name: "C", writes: []ComponentID{c2}},
		&fnSystem{name: "D", writes: []ComponentID{c3}},
		&fnSystem{name: "E", writes: []ComponentID{c4}},
	}

	levels := dependencyLevels(systems)
	// A -> level 0; B (conflicts with A) -> level 1; C (conflicts with B) -> 2;
	// D and E independent of everything -> level 0 with A.
	if len(levels) != 3 {
		t.Fatalf("expected 3 levels, got %d", len(levels))
	}
	names := func(level []int) []string {
		out := make([]string, len(level))
		for i, idx := range level {
			out[i] = systems[idx].Name()
		}
		return out
	}
	lvl0 := names(levels[0])
	if len(lvl0) != 3 || lvl0[0] != "A" || lvl0[1] != "D" || lvl0[2] != "E" {
		t.Fatalf("level 0 = %v, want [A D E]", lvl0)
	}
	if got := names(levels[1]); len(got) != 1 || got[0] != "B" {
		t.Fatalf("level 1 = %v, want [B]", got)
	}
	if got := names(levels[2]); len(got) != 1 || got[0] != "C" {
		t.Fatalf("level 2 = %v, want [C]", got)
	}
}

func TestConflicts(t *testing.T) {
	c1, c2 := ComponentID(1), ComponentID(2)
	a := &fnSystem{name: "a", writes: []ComponentID{c1}}
	b := &fnSystem{name: "b", reads: []ComponentID{c1}}
	c := &fnSystem{name: "c", reads: []ComponentID{c2}}
	if !conflicts(a, b) {
		t.Fatal("write-read conflict not detected")
	}
	if conflicts(a, c) {
		t.Fatal("independent systems reported as conflicting")
	}
	if !conflicts(a, a) {
		t.Fatal("write-write self conflict not detected")
	}
}

// depModel expresses a tick as two systems with a write->read dependency:
// system "mul" doubles each entity's accumulator, then system "add" adds one.
// Because "add" reads what "mul" writes, the scheduler must run "mul" first,
// giving a deterministic final value of (1*2)+1 = 3 per entity.
type depModel struct {
	accID  ComponentID
	accCol *Column[int]
}

func (m *depModel) Metadata() model.Metadata {
	return model.Metadata{ID: "dep", Name: "Dep", Version: "1", Mode: model.ModeTick}
}

func (m *depModel) Initialize(ctx context.Context, w *World) error {
	m.accID, m.accCol = RegisterComponent[int](w.Components, "dep.acc")
	for i := 0; i < 1000; i++ {
		m.accCol.Set(w.Entities.Create(), 1)
	}
	return nil
}

func (m *depModel) Systems() []System {
	return []System{&depMulSystem{m: m}, &depAddSystem{m: m}}
}

type depMulSystem struct{ m *depModel }

func (s *depMulSystem) Name() string          { return "mul" }
func (s *depMulSystem) Reads() []ComponentID  { return []ComponentID{s.m.accID} }
func (s *depMulSystem) Writes() []ComponentID { return []ComponentID{s.m.accID} }
func (s *depMulSystem) Run(ctx context.Context, w *World, shard []EntityID) error {
	for _, e := range shard {
		v, _ := s.m.accCol.GetShard(e)
		s.m.accCol.SetShard(e, v*2)
	}
	return nil
}

type depAddSystem struct{ m *depModel }

func (s *depAddSystem) Name() string          { return "add" }
func (s *depAddSystem) Reads() []ComponentID  { return []ComponentID{s.m.accID} }
func (s *depAddSystem) Writes() []ComponentID { return []ComponentID{s.m.accID} }
func (s *depAddSystem) Run(ctx context.Context, w *World, shard []EntityID) error {
	for _, e := range shard {
		v, _ := s.m.accCol.GetShard(e)
		s.m.accCol.SetShard(e, v+1)
	}
	return nil
}

func depSum(w *World, col *Column[int]) int {
	sum := 0
	col.Each(func(_ EntityID, v int) { sum += v })
	return sum
}

func TestSystemDependencyOrdering(t *testing.T) {
	sim, err := New(context.Background(), Config{ID: "s", Seed: 1, Mode: model.ModeTick, MaxTicks: 1, Workers: 4}, &depModel{})
	if err != nil {
		t.Fatal(err)
	}
	if err := sim.Run(context.Background()); err != nil {
		t.Fatal(err)
	}
	m := sim.model.(*depModel)
	if got := depSum(sim.World(), m.accCol); got != 3*1000 {
		t.Fatalf("sum = %d, want %d (dependency ordering violated)", got, 3*1000)
	}
}

func TestSystemDependencyParallelDeterminism(t *testing.T) {
	run := func(workers int) int {
		m := &depModel{}
		sim, err := New(context.Background(), Config{ID: "s", Seed: 1, Mode: model.ModeTick, MaxTicks: 1, Workers: workers}, m)
		if err != nil {
			t.Fatal(err)
		}
		if err := sim.Run(context.Background()); err != nil {
			t.Fatal(err)
		}
		return depSum(sim.World(), m.accCol)
	}
	if a, b := run(1), run(8); a != b {
		t.Fatalf("parallel determinism violated: workers=1 -> %d, workers=8 -> %d", a, b)
	}
}
