package simulation

import (
	"container/heap"
	"encoding/json"
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

// pushRaw pushes an event exactly as given, without assigning an ID or sequence
// number. Used when restoring a snapshot to preserve the original IDs/sequences.
func (q *EventQueue) pushRaw(e Event) {
	heap.Push(&q.h, e)
}

// eventSnapshot is the serializable form of an Event. The payload is stored as
// opaque JSON; models with typed payloads must re-decode it on restore (payload
// codec registration is future work).
type eventSnapshot struct {
	ID       EventID         `json:"id"`
	Type     string          `json:"type"`
	Time     float64         `json:"time"`
	Priority int             `json:"priority"`
	Seq      uint64          `json:"seq"`
	Source   EntityID        `json:"source"`
	Target   EntityID        `json:"target"`
	Payload  json.RawMessage `json:"payload,omitempty"`
}

// eventQueueState is the serializable form of the event queue, including the
// ID and sequence counters so future scheduling continues deterministically.
type eventQueueState struct {
	Events []eventSnapshot `json:"events"`
	NextID EventID         `json:"next_id"`
	Seq    uint64          `json:"seq"`
}

func (q *EventQueue) snapshot() (eventQueueState, error) {
	q.mu.Lock()
	defer q.mu.Unlock()
	st := eventQueueState{NextID: q.next, Seq: q.seq}
	st.Events = make([]eventSnapshot, 0, len(q.h))
	for _, e := range q.h {
		es := eventSnapshot{
			ID:       e.ID,
			Type:     e.Type,
			Time:     e.Time,
			Priority: e.Priority,
			Seq:      e.Seq,
			Source:   e.Source,
			Target:   e.Target,
		}
		if e.Payload != nil {
			b, err := json.Marshal(e.Payload)
			if err != nil {
				return eventQueueState{}, err
			}
			es.Payload = b
		}
		st.Events = append(st.Events, es)
	}
	return st, nil
}

func (q *EventQueue) restore(st eventQueueState) {
	q.mu.Lock()
	defer q.mu.Unlock()
	q.h = q.h[:0]
	q.next = st.NextID
	q.seq = st.Seq
	for _, es := range st.Events {
		var payload any
		if len(es.Payload) > 0 {
			payload = json.RawMessage(es.Payload)
		}
		q.pushRaw(Event{
			ID:       es.ID,
			Type:     es.Type,
			Time:     es.Time,
			Priority: es.Priority,
			Seq:      es.Seq,
			Source:   es.Source,
			Target:   es.Target,
			Payload:  payload,
		})
	}
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
