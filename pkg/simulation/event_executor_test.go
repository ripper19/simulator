package simulation

import (
	"context"
	"fmt"
	"testing"

	"github.com/ripper19/simulator/pkg/model"
)

// orderedEventModel schedules a fixed set of events out of order and records
// the sequence in which they are handled.
type orderedEventModel struct {
	trace []string
}

func (m *orderedEventModel) Metadata() model.Metadata {
	return model.Metadata{ID: "ordered", Name: "Ordered", Version: "1", Mode: model.ModeEvent}
}

func (m *orderedEventModel) Initialize(ctx context.Context, w *World) error {
	w.ScheduleAt(5, "e", nil)
	w.ScheduleAt(1, "e", nil)
	w.ScheduleAt(3, "e", nil)
	w.ScheduleAt(2, "e", nil)
	w.ScheduleAt(4, "e", nil)
	return nil
}

func (m *orderedEventModel) HandleEvent(ctx context.Context, w *World, e Event) error {
	m.trace = append(m.trace, fmt.Sprintf("%.0f:%s", e.Time, e.Type))
	return nil
}

func TestEventModeTimeOrdering(t *testing.T) {
	m := &orderedEventModel{}
	sim, err := New(context.Background(), Config{ID: "s", Seed: 1, Mode: model.ModeEvent}, m)
	if err != nil {
		t.Fatal(err)
	}
	if err := sim.Run(context.Background()); err != nil {
		t.Fatal(err)
	}
	want := []string{"1:e", "2:e", "3:e", "4:e", "5:e"}
	if len(m.trace) != len(want) {
		t.Fatalf("trace length %d, want %d (%v)", len(m.trace), len(want), m.trace)
	}
	for i := range want {
		if m.trace[i] != want[i] {
			t.Fatalf("trace[%d] = %q, want %q (trace %v)", i, m.trace[i], want[i], m.trace)
		}
	}
	if sim.State() != StateCompleted {
		t.Fatalf("state = %s, want completed", sim.State())
	}
	if sim.World().Clock.Time() != 5 {
		t.Fatalf("final clock time = %f, want 5", sim.World().Clock.Time())
	}
}

// priorityModel schedules events at identical time with different priorities.
type priorityModel struct{ trace []string }

func (m *priorityModel) Metadata() model.Metadata {
	return model.Metadata{ID: "prio", Name: "Prio", Version: "1", Mode: model.ModeEvent}
}

func (m *priorityModel) Initialize(ctx context.Context, w *World) error {
	w.ScheduleAtPriority(0, 0, "low", nil)
	w.ScheduleAtPriority(0, 10, "high", nil)
	w.ScheduleAtPriority(0, 5, "mid", nil)
	return nil
}

func (m *priorityModel) HandleEvent(ctx context.Context, w *World, e Event) error {
	m.trace = append(m.trace, e.Type)
	return nil
}

func TestEventModeSameTimePriority(t *testing.T) {
	m := &priorityModel{}
	sim, err := New(context.Background(), Config{ID: "s", Seed: 1, Mode: model.ModeEvent}, m)
	if err != nil {
		t.Fatal(err)
	}
	if err := sim.Run(context.Background()); err != nil {
		t.Fatal(err)
	}
	want := []string{"high", "mid", "low"}
	for i := range want {
		if m.trace[i] != want[i] {
			t.Fatalf("trace[%d] = %q, want %q (trace %v)", i, m.trace[i], want[i], m.trace)
		}
	}
}

// cascadeModel schedules immediate and delayed events from within event
// handling, building a deterministic cascade.
type cascadeModel struct {
	depth int
	trace []string
}

func (m *cascadeModel) Metadata() model.Metadata {
	return model.Metadata{ID: "cascade", Name: "Cascade", Version: "1", Mode: model.ModeEvent}
}

func (m *cascadeModel) Initialize(ctx context.Context, w *World) error {
	w.ScheduleNow("fire", 0)
	return nil
}

func (m *cascadeModel) HandleEvent(ctx context.Context, w *World, e Event) error {
	step := e.Payload.(int)
	m.trace = append(m.trace, fmt.Sprintf("%.1f:%d", e.Time, step))
	if step < m.depth {
		w.ScheduleIn(1.0, "fire", step+1) // delayed child
		w.ScheduleNow("fire", step+1)     // immediate child
	}
	return nil
}

func TestEventModeImmediateAndDelayed(t *testing.T) {
	m := &cascadeModel{depth: 2}
	sim, err := New(context.Background(), Config{ID: "s", Seed: 1, Mode: model.ModeEvent}, m)
	if err != nil {
		t.Fatal(err)
	}
	if err := sim.Run(context.Background()); err != nil {
		t.Fatal(err)
	}
	// Expected cascade for depth=2 (each fire schedules a delayed child at t+1
	// and an immediate child at t):
	//   fire(0)@0 -> fire(1)@0 (immediate) -> fire(2)@0 (immediate, leaf)
	//        and fire(1)@1 (delayed) -> fire(2)@1 (immediate) + fire(2)@2 (delayed)
	//        and fire(2)@1 (delayed, from the first immediate fire(1))
	want := []string{"0.0:0", "0.0:1", "0.0:2", "1.0:1", "1.0:2", "1.0:2", "2.0:2"}
	if len(m.trace) != len(want) {
		t.Fatalf("trace length %d, want %d (%v)", len(m.trace), len(want), m.trace)
	}
	for i := range want {
		if m.trace[i] != want[i] {
			t.Fatalf("trace[%d] = %q, want %q (trace %v)", i, m.trace[i], want[i], m.trace)
		}
	}
}

// maxTimeModel schedules events well beyond the configured MaxTime.
type maxTimeModel struct {
	trace []string
}

func (m *maxTimeModel) Metadata() model.Metadata {
	return model.Metadata{ID: "maxtime", Name: "MaxTime", Version: "1", Mode: model.ModeEvent}
}

func (m *maxTimeModel) Initialize(ctx context.Context, w *World) error {
	for i := 0; i < 10; i++ {
		w.ScheduleAt(float64(i), "e", i)
	}
	return nil
}

func (m *maxTimeModel) HandleEvent(ctx context.Context, w *World, e Event) error {
	m.trace = append(m.trace, fmt.Sprintf("%.0f", e.Time))
	return nil
}

func TestEventModeMaxTimeBoundary(t *testing.T) {
	m := &maxTimeModel{}
	sim, err := New(context.Background(), Config{ID: "s", Seed: 1, Mode: model.ModeEvent, MaxTime: 5}, m)
	if err != nil {
		t.Fatal(err)
	}
	if err := sim.Run(context.Background()); err != nil {
		t.Fatal(err)
	}
	// Events at times 0..4 are processed; the event at time 5 must not be.
	want := []string{"0", "1", "2", "3", "4"}
	if len(m.trace) != len(want) {
		t.Fatalf("trace length %d, want %d (%v)", len(m.trace), len(want), m.trace)
	}
	for i := range want {
		if m.trace[i] != want[i] {
			t.Fatalf("trace[%d] = %q, want %q (trace %v)", i, m.trace[i], want[i], m.trace)
		}
	}
}

func TestEventModeEmptyQueue(t *testing.T) {
	type emptyModel struct{}
	m := &emptyModel{}
	// Reuse the metadata from cascadeModel via a small local model.
	_ = m
	em := &noEventModel{}
	sim, err := New(context.Background(), Config{ID: "s", Seed: 1, Mode: model.ModeEvent}, em)
	if err != nil {
		t.Fatal(err)
	}
	if err := sim.Run(context.Background()); err != nil {
		t.Fatal(err)
	}
	if sim.State() != StateCompleted {
		t.Fatalf("state = %s, want completed", sim.State())
	}
	if sim.Ticks() != 0 {
		t.Fatalf("steps = %d, want 0", sim.Ticks())
	}
}

// noEventModel schedules nothing; the queue is empty from the start.
type noEventModel struct{}

func (*noEventModel) Metadata() model.Metadata {
	return model.Metadata{ID: "noevent", Name: "NoEvent", Version: "1", Mode: model.ModeEvent}
}

func (*noEventModel) Initialize(ctx context.Context, w *World) error { return nil }

func (*noEventModel) HandleEvent(ctx context.Context, w *World, e Event) error { return nil }

func TestEventModeDeterministicReplay(t *testing.T) {
	run := func() []string {
		m := &cascadeModel{depth: 5}
		sim, err := New(context.Background(), Config{ID: "s", Seed: 42, Mode: model.ModeEvent}, m)
		if err != nil {
			t.Fatal(err)
		}
		if err := sim.Run(context.Background()); err != nil {
			t.Fatal(err)
		}
		return m.trace
	}
	a := run()
	b := run()
	if len(a) != len(b) {
		t.Fatalf("replay length differs: %d vs %d", len(a), len(b))
	}
	for i := range a {
		if a[i] != b[i] {
			t.Fatalf("replay diverged at %d: %q vs %q", i, a[i], b[i])
		}
	}
}
