package simulation

import (
	"context"

	"github.com/ripper19/simulator/pkg/model"
)

// executor is the internal seam between the runtime and a specific execution
// strategy (discrete-tick or discrete-event). It abstracts a single unit of
// work so the runtime loop is identical regardless of mode.
type executor interface {
	// step performs one unit of work and returns false when there is nothing
	// left to do (e.g. no events remain in event mode).
	step(ctx context.Context, w *World) (bool, error)
}

// tickExecutor drives a TickModel one tick per step. A step is always
// available, so step returns true until an external limit stops the run.
type tickExecutor struct {
	m TickModel
}

func (e tickExecutor) step(ctx context.Context, w *World) (bool, error) {
	if err := e.m.Step(ctx, w); err != nil {
		return false, err
	}
	w.Clock.Advance()
	return true, nil
}

// eventExecutor drives an EventModel by popping events until the queue is
// empty. It is introduced in the discrete-event phase; see phase-2.
type eventExecutor struct {
	m EventModel
}

func (e eventExecutor) step(ctx context.Context, w *World) (bool, error) {
	ev, ok := w.Events.Pop()
	if !ok {
		return false, nil
	}
	w.Clock.AdvanceTime(ev.Time)
	if err := e.m.HandleEvent(ctx, w, ev); err != nil {
		return false, err
	}
	return true, nil
}

// newExecutor builds the executor for a model based on the configured mode.
func newExecutor(cfg Config, m Model) (executor, error) {
	switch cfg.Mode {
	case model.ModeTick:
		tm, ok := m.(TickModel)
		if !ok {
			return nil, ErrBadMode
		}
		return tickExecutor{m: tm}, nil
	case model.ModeEvent:
		em, ok := m.(EventModel)
		if !ok {
			return nil, ErrBadMode
		}
		return eventExecutor{m: em}, nil
	default:
		return nil, ErrBadMode
	}
}
