package simulation

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/ripper19/simulator/pkg/model"
)

// typedPayload is a registered payload type used to verify type-preserving
// snapshot/restore of event payloads.
type typedPayload struct {
	Value int    `json:"value"`
	Name  string `json:"name"`
}

func TestPayloadTypeRoundTrip(t *testing.T) {
	w := NewWorld("w", 1)
	w.RegisterPayloadType(typedPayload{})
	w.Events.Push(Event{Type: "e", Time: 5, Payload: typedPayload{Value: 42, Name: "x"}})
	// An unregistered payload (plain int) must restore as opaque JSON.
	w.Events.Push(Event{Type: "e2", Time: 6, Payload: 7})

	snap, err := w.Snapshot()
	if err != nil {
		t.Fatal(err)
	}

	w2 := NewWorld("w", 1)
	w2.RegisterPayloadType(typedPayload{})
	if err := w2.Restore(snap); err != nil {
		t.Fatal(err)
	}

	ev, ok := w2.Events.Pop()
	if !ok {
		t.Fatal("expected first event")
	}
	p, ok := ev.Payload.(typedPayload)
	if !ok || p.Value != 42 || p.Name != "x" {
		t.Fatalf("typed payload not preserved: %#v", ev.Payload)
	}

	ev, _ = w2.Events.Pop()
	if _, ok := ev.Payload.(json.RawMessage); !ok {
		t.Fatalf("unregistered payload should restore as json.RawMessage, got %T", ev.Payload)
	}
}

func TestTagsResourcesSnapshot(t *testing.T) {
	w := NewWorld("w", 1)
	e := w.Entities.Create()
	tag := w.Tags.Register("movable")
	w.TagStore.Add(e, tag)
	SetResource(w.Resources, map[string]int{"k": 1})

	snap, err := w.Snapshot()
	if err != nil {
		t.Fatal(err)
	}

	w2 := NewWorld("w", 1)
	SetResource(w2.Resources, map[string]int{}) // register the resource type before restore
	if err := w2.Restore(snap); err != nil {
		t.Fatal(err)
	}

	if !w2.TagStore.Has(e, tag) {
		t.Fatal("tag not restored")
	}
	if w2.Tags.Name(tag) != "movable" {
		t.Fatalf("tag name not restored: %q", w2.Tags.Name(tag))
	}
	r, ok := GetResource[map[string]int](w2.Resources)
	if !ok || r["k"] != 1 {
		t.Fatalf("resource not restored: %v %v", r, ok)
	}
}

type panicModel struct{ countingModel }

func (m *panicModel) Step(ctx context.Context, w *World) error {
	panic("boom")
}

func TestModelPanicRecovery(t *testing.T) {
	sim, err := New(context.Background(), Config{ID: "s", Seed: 1, Mode: model.ModeTick}, &panicModel{})
	if err != nil {
		t.Fatal(err)
	}
	err = sim.Run(context.Background())
	if err == nil || !strings.Contains(err.Error(), "panic") {
		t.Fatalf("expected panic error, got %v", err)
	}
	if sim.State() != StateFailed {
		t.Fatalf("state = %s, want failed", sim.State())
	}
}

func TestModelPanicWaitDoesNotHang(t *testing.T) {
	sim, err := New(context.Background(), Config{ID: "s", Seed: 1, Mode: model.ModeTick}, &panicModel{})
	if err != nil {
		t.Fatal(err)
	}
	if err := sim.Start(context.Background()); err != nil {
		t.Fatal(err)
	}
	done := make(chan error, 1)
	go func() { done <- sim.Wait() }()
	select {
	case err := <-done:
		if err == nil || !strings.Contains(err.Error(), "panic") {
			t.Fatalf("expected panic error, got %v", err)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("Wait() hung after model panic")
	}
}

// pastModel schedules an event into the past, which must be rejected.
type pastModel struct{}

func (*pastModel) Metadata() model.Metadata {
	return model.Metadata{ID: "past", Name: "Past", Version: "1", Mode: model.ModeEvent}
}

func (*pastModel) Initialize(ctx context.Context, w *World) error {
	w.ScheduleAt(5, "e", nil)
	return nil
}

func (*pastModel) HandleEvent(ctx context.Context, w *World, e Event) error {
	w.ScheduleAt(1, "e", nil) // into the past
	return nil
}

func TestEventInPastRejected(t *testing.T) {
	sim, err := New(context.Background(), Config{ID: "s", Seed: 1, Mode: model.ModeEvent}, &pastModel{})
	if err != nil {
		t.Fatal(err)
	}
	err = sim.Run(context.Background())
	if err == nil {
		t.Fatal("expected past-event error")
	}
	if sim.State() != StateFailed {
		t.Fatalf("state = %s, want failed", sim.State())
	}
}

// blockingModel's system blocks until the run context is cancelled, then
// returns ctx.Err() — deterministically reproducing the Stop-mid-step race
// where a step surfaces context.Canceled.
type blockingModel struct {
	accID  ComponentID
	accCol *Column[int]
}

func (m *blockingModel) Metadata() model.Metadata {
	return model.Metadata{ID: "blocking", Name: "Blocking", Version: "1", Mode: model.ModeTick}
}

func (m *blockingModel) Initialize(ctx context.Context, w *World) error {
	m.accID, m.accCol = RegisterComponent[int](w.Components, "block.acc")
	for i := 0; i < 1000; i++ {
		m.accCol.Set(w.Entities.Create(), 0)
	}
	return nil
}

func (m *blockingModel) Systems() []System {
	return []System{&blockingSystem{m: m}}
}

type blockingSystem struct{ m *blockingModel }

func (s *blockingSystem) Name() string          { return "block" }
func (s *blockingSystem) Reads() []ComponentID  { return []ComponentID{s.m.accID} }
func (s *blockingSystem) Writes() []ComponentID { return []ComponentID{s.m.accID} }
func (s *blockingSystem) Run(ctx context.Context, w *World, shard []EntityID) error {
	<-ctx.Done()
	return ctx.Err()
}

func TestStopMidStepIsStoppedNotFailed(t *testing.T) {
	m := &blockingModel{}
	sim, err := New(context.Background(), Config{ID: "s", Seed: 1, Mode: model.ModeTick, Workers: 2}, m)
	if err != nil {
		t.Fatal(err)
	}
	if err := sim.Start(context.Background()); err != nil {
		t.Fatal(err)
	}
	// Give the run loop time to enter the blocking step.
	time.Sleep(20 * time.Millisecond)
	if err := sim.Stop(); err != nil {
		t.Fatal(err)
	}
	if err := sim.Wait(); err != nil {
		t.Fatalf("Stop surfaced an error (%v); a cancellation mid-step must not be a failure (state=%s)", err, sim.State())
	}
	if sim.State() != StateStopped {
		t.Fatalf("state = %s, want stopped", sim.State())
	}
}
