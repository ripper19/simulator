package simulation

import (
	"container/heap"
	"sync"
)

// EventID is a unique, monotonically increasing identifier for an event.
type EventID uint64

// Event is a domain-agnostic simulation event. The engine assigns no meaning
// to Type or Payload; they are interpreted entirely by the model.
type Event struct {
	// ID is assigned by the event queue when the event is scheduled.
	ID EventID
	// Type is a model-defined event kind, e.g. "arrival".
	Type string
	// Time is the simulation time at which the event fires. Events are ordered
	// by time first, so this is the primary ordering key.
	Time float64
	// Priority orders events scheduled for the same time; higher fires first.
	Priority int
	// Seq is the queue-assigned insertion sequence, used to break ties in a
	// way that depends only on scheduling order, keeping the queue
	// deterministic.
	Seq uint64
	// Source and Target are optional entity references (0 means none).
	Source EntityID
	Target EntityID
	// Payload carries model-defined data.
	Payload any
}

// less reports whether a should fire before b, establishing a total order.
func eventLess(a, b Event) bool {
	if a.Time != b.Time {
		return a.Time < b.Time
	}
	if a.Priority != b.Priority {
		return a.Priority > b.Priority
	}
	if a.Seq != b.Seq {
		return a.Seq < b.Seq
	}
	return a.ID < b.ID
}

type eventHeap []Event

func (h eventHeap) Len() int           { return len(h) }
func (h eventHeap) Less(i, j int) bool { return eventLess(h[i], h[j]) }
func (h eventHeap) Swap(i, j int)      { h[i], h[j] = h[j], h[i] }
func (h *eventHeap) Push(x any)        { *h = append(*h, x.(Event)) }
func (h *eventHeap) Pop() any          { old := *h; n := len(old); e := old[n-1]; *h = old[:n-1]; return e }

// EventQueue is a priority queue of events with deterministic ordering by
// (time, priority descending, sequence, id). It is safe for concurrent use.
type EventQueue struct {
	mu   sync.Mutex
	h    eventHeap
	next EventID
	seq  uint64
}

// NewEventQueue returns an empty event queue.
func NewEventQueue() *EventQueue {
	return &EventQueue{}
}

// Push schedules an event. If the event's ID is zero, the queue assigns a
// fresh unique ID; the queue always assigns a fresh sequence number, so
// ordering depends only on scheduling order.
func (q *EventQueue) Push(e Event) Event {
	q.mu.Lock()
	defer q.mu.Unlock()
	if e.ID == 0 {
		q.next++
		e.ID = q.next
	}
	q.seq++
	e.Seq = q.seq
	heap.Push(&q.h, e)
	return e
}

// Peek returns the next event to fire without removing it.
func (q *EventQueue) Peek() (Event, bool) {
	q.mu.Lock()
	defer q.mu.Unlock()
	if len(q.h) == 0 {
		return Event{}, false
	}
	return q.h[0], true
}

// Pop removes and returns the next event to fire.
func (q *EventQueue) Pop() (Event, bool) {
	q.mu.Lock()
	defer q.mu.Unlock()
	if len(q.h) == 0 {
		return Event{}, false
	}
	return heap.Pop(&q.h).(Event), true
}

// Len returns the number of queued events.
func (q *EventQueue) Len() int {
	q.mu.Lock()
	defer q.mu.Unlock()
	return len(q.h)
}
