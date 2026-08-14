package simulation

import (
	"container/heap"
	"encoding/json"
	"fmt"
	"reflect"
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
// JSON. Payload types registered via World.RegisterPayloadType are wrapped in an
// envelope that preserves their Go type on restore; unregistered types restore
// as opaque json.RawMessage.
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

// payloadEnvelope wraps a registered payload type with its Go type name so it
// can be reconstructed on restore.
type payloadEnvelope struct {
	Type string          `json:"__t"`
	Data json.RawMessage `json:"data"`
}

// marshalPayload serializes a payload. Registered types are wrapped in an
// envelope; everything else is serialized as plain JSON.
func marshalPayload(p any, types map[string]reflect.Type) (json.RawMessage, error) {
	if p == nil {
		return nil, nil
	}
	name := reflect.TypeOf(p).String()
	if _, ok := types[name]; ok {
		data, err := json.Marshal(p)
		if err != nil {
			return nil, err
		}
		return json.Marshal(payloadEnvelope{Type: name, Data: data})
	}
	return json.Marshal(p)
}

// unmarshalPayload reconstructs a payload. Enveloped payloads are decoded back
// into their registered Go type; others become opaque json.RawMessage.
func unmarshalPayload(raw json.RawMessage, types map[string]reflect.Type) (any, error) {
	if len(raw) == 0 {
		return nil, nil
	}
	var env payloadEnvelope
	if err := json.Unmarshal(raw, &env); err == nil && env.Type != "" {
		t, ok := types[env.Type]
		if !ok {
			return nil, fmt.Errorf("simulation: snapshot payload type %q is not registered", env.Type)
		}
		v := reflect.New(t)
		if err := json.Unmarshal(env.Data, v.Interface()); err != nil {
			return nil, fmt.Errorf("simulation: decode payload %q: %w", env.Type, err)
		}
		return v.Elem().Interface(), nil
	}
	return json.RawMessage(raw), nil
}

func (q *EventQueue) snapshot(payloadTypes map[string]reflect.Type) (eventQueueState, error) {
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
			b, err := marshalPayload(e.Payload, payloadTypes)
			if err != nil {
				return eventQueueState{}, err
			}
			es.Payload = b
		}
		st.Events = append(st.Events, es)
	}
	return st, nil
}

func (q *EventQueue) restore(st eventQueueState, payloadTypes map[string]reflect.Type) error {
	q.mu.Lock()
	defer q.mu.Unlock()
	q.h = q.h[:0]
	q.next = st.NextID
	q.seq = st.Seq
	for _, es := range st.Events {
		payload, err := unmarshalPayload(es.Payload, payloadTypes)
		if err != nil {
			return err
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
	return nil
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
