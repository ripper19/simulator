package simulation

import (
	"context"
	"testing"

	"github.com/ripper19/simulator/pkg/model"
)

func TestSnapshotValidateAndTamper(t *testing.T) {
	sim, err := New(context.Background(), Config{ID: "s", Seed: 1, Mode: model.ModeTick}, &countingModel{})
	if err != nil {
		t.Fatal(err)
	}
	snap, err := sim.Snapshot()
	if err != nil {
		t.Fatal(err)
	}
	if err := snap.Validate(); err != nil {
		t.Fatalf("valid snapshot failed validation: %v", err)
	}
	// Tamper with the clock; the checksum must catch it.
	snap.Tick++
	if err := snap.Validate(); err == nil {
		t.Fatal("tampered snapshot passed validation")
	}
}

func TestSnapshotSchemaVersionMismatch(t *testing.T) {
	sim, _ := New(context.Background(), Config{ID: "s", Seed: 1, Mode: model.ModeTick}, &countingModel{})
	snap, _ := sim.Snapshot()
	snap.SchemaVersion = 999
	if err := snap.Validate(); err == nil {
		t.Fatal("wrong schema version passed validation")
	}
}

func TestRestoreRejectsModelMismatch(t *testing.T) {
	a, _ := New(context.Background(), Config{ID: "s", Seed: 1, Mode: model.ModeTick}, &countingModel{})
	snap, _ := a.Snapshot()
	// A different model (also tick mode, different ID) must be rejected.
	b, err := New(context.Background(), Config{ID: "s", Seed: 1, Mode: model.ModeTick}, &countingModel{})
	if err != nil {
		t.Fatal(err)
	}
	_ = b
	// countingModel always reports the same ID; simulate a mismatch by mutating
	// the snapshot's model ID.
	snap.ModelID = "other-model"
	if err := a.Restore(snap); err == nil {
		t.Fatal("restore with mismatched model was accepted")
	}
}

func TestRestoreRejectsSeedMismatch(t *testing.T) {
	a, _ := New(context.Background(), Config{ID: "s", Seed: 1, Mode: model.ModeTick}, &countingModel{})
	snap, _ := a.Snapshot()
	b, err := New(context.Background(), Config{ID: "s", Seed: 2, Mode: model.ModeTick}, &countingModel{})
	if err != nil {
		t.Fatal(err)
	}
	if err := b.Restore(snap); err == nil {
		t.Fatal("restore with mismatched seed was accepted")
	}
}

func TestEventQueueSnapshotRoundTrip(t *testing.T) {
	q := NewEventQueue()
	q.Push(Event{Type: "a", Time: 1})
	q.Push(Event{Type: "b", Time: 2, Priority: 5})
	q.Push(Event{Type: "c", Time: 1, Priority: 1, Payload: map[string]any{"k": "v"}})

	st, err := q.snapshot()
	if err != nil {
		t.Fatal(err)
	}
	q2 := NewEventQueue()
	q2.restore(st)

	for {
		e1, ok1 := q.Pop()
		e2, ok2 := q2.Pop()
		if ok1 != ok2 {
			t.Fatal("restored queue length differs")
		}
		if !ok1 {
			break
		}
		if e1.Type != e2.Type || e1.Time != e2.Time || e1.Priority != e2.Priority || e1.ID != e2.ID || e1.Seq != e2.Seq {
			t.Fatalf("restored event differs: %+v vs %+v", e1, e2)
		}
	}
}

func TestSnapshotRoundTripWorld(t *testing.T) {
	// Build a world with entities, components, tags, and events; snapshot it;
	// restore into an identical world; snapshots must be identical.
	m := &countingModel{}
	sim, err := New(context.Background(), Config{ID: "s", Seed: 42, Mode: model.ModeTick}, m)
	if err != nil {
		t.Fatal(err)
	}
	sim.RunN(context.Background(), 5)

	snap, err := sim.Snapshot()
	if err != nil {
		t.Fatal(err)
	}

	// Restore into the same simulation and re-snapshot; must be byte-identical.
	if err := sim.Restore(snap); err != nil {
		t.Fatal(err)
	}
	snap2, err := sim.Snapshot()
	if err != nil {
		t.Fatal(err)
	}
	if snap.Checksum != snap2.Checksum {
		t.Fatal("snapshot round-trip produced different state")
	}
}
