package simulation

import "testing"

func TestEventQueueOrdering(t *testing.T) {
	q := NewEventQueue()
	// Later time must fire after earlier time regardless of insertion order.
	q.Push(Event{Type: "late", Time: 10})
	q.Push(Event{Type: "early", Time: 5})
	// Same time: higher priority first.
	q.Push(Event{Type: "low-prio", Time: 5, Priority: 1})
	q.Push(Event{Type: "high-prio", Time: 5, Priority: 10})

	want := []string{"high-prio", "low-prio", "early", "late"}
	for i, w := range want {
		e, ok := q.Pop()
		if !ok {
			t.Fatalf("queue emptied at %d", i)
		}
		if e.Type != w {
			t.Fatalf("step %d: got %q, want %q", i, e.Type, w)
		}
	}
	if _, ok := q.Pop(); ok {
		t.Fatal("queue should be empty")
	}
}

func TestEventQueueDeterministicTiebreak(t *testing.T) {
	// Events at identical (time, priority) must be ordered by insertion sequence.
	build := func() []string {
		q := NewEventQueue()
		q.Push(Event{Type: "a", Time: 1})
		q.Push(Event{Type: "b", Time: 1})
		q.Push(Event{Type: "c", Time: 1})
		var out []string
		for {
			e, ok := q.Pop()
			if !ok {
				break
			}
			out = append(out, e.Type)
		}
		return out
	}
	r1 := build()
	r2 := build()
	for i := range r1 {
		if r1[i] != r2[i] {
			t.Fatalf("non-deterministic tiebreak at %d: %v vs %v", i, r1, r2)
		}
	}
}

func TestEventQueueAssignsIDs(t *testing.T) {
	q := NewEventQueue()
	e1 := q.Push(Event{Type: "a"})
	e2 := q.Push(Event{Type: "b"})
	if e1.ID == 0 || e2.ID == 0 || e1.ID == e2.ID {
		t.Fatalf("queue must assign unique non-zero IDs: %d %d", e1.ID, e2.ID)
	}
	if e1.Seq >= e2.Seq {
		t.Fatalf("sequence must be monotonic: %d %d", e1.Seq, e2.Seq)
	}
}
