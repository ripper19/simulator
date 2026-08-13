package simulation

// This file provides ergonomic event-scheduling helpers on the World. They are
// thin wrappers over the event queue and clock and carry no domain meaning;
// models use them to schedule immediate, delayed, and prioritized events.

// ScheduleAt schedules an event of the given type to fire at absolute
// simulation time t, returning the queued event (with its assigned ID and
// sequence).
func (w *World) ScheduleAt(t float64, typ string, payload any) Event {
	return w.Events.Push(Event{Type: typ, Time: t, Payload: payload})
}

// ScheduleAtPriority schedules an event at absolute time t with the given
// priority. Among events at the same time, higher priority fires first.
func (w *World) ScheduleAtPriority(t float64, priority int, typ string, payload any) Event {
	return w.Events.Push(Event{Type: typ, Time: t, Priority: priority, Payload: payload})
}

// ScheduleIn schedules an event to fire delay time units after the current
// clock time (a delayed event).
func (w *World) ScheduleIn(delay float64, typ string, payload any) Event {
	return w.Events.Push(Event{Type: typ, Time: w.Clock.Time() + delay, Payload: payload})
}

// ScheduleNow schedules an event to fire at the current clock time (an
// immediate event). Because it carries the current time, it fires before any
// already-queued event at a later time but after any earlier-scheduled event.
func (w *World) ScheduleNow(typ string, payload any) Event {
	return w.Events.Push(Event{Type: typ, Time: w.Clock.Time(), Payload: payload})
}
