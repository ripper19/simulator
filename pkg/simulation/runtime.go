package simulation

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
	"sync/atomic"
	"time"

	"github.com/ripper19/simulator/pkg/model"
)

// Simulation binds a model to a World and drives execution according to a
// Config. It supports start/pause/resume/stop, single-step, bounded runs
// (RunN), and predicate-limited runs (RunUntil), plus context cancellation for
// clean termination.
//
// A Simulation is not safe to copy after construction. All state transitions
// are guarded by a mutex; execution is single-threaded by design in this
// phase (parallel execution is a later phase), which keeps runs deterministic.
type Simulation struct {
	mu    sync.Mutex
	cond  *sync.Cond
	cfg   Config
	model Model
	world *World
	exec  executor

	state State
	err   error
	steps atomic.Uint64

	stepObserver func(time.Duration)

	cancel context.CancelFunc
	done   chan struct{}
}

// New constructs a Simulation, builds its World, and runs the model's
// Initialize. It returns an error if the config mode does not match the model
// or if initialization fails. The returned simulation is in StateIdle.
func New(ctx context.Context, cfg Config, m Model) (*Simulation, error) {
	exec, err := newExecutor(cfg, m)
	if err != nil {
		return nil, err
	}
	world := NewWorld(cfg.ID, cfg.Seed)
	world.Meta = Metadata{
		SimulationID: cfg.ID,
		ModelID:      m.Metadata().ID,
		ModelVersion: m.Metadata().Version,
		Seed:         cfg.Seed,
		Mode:         cfg.Mode,
	}
	if err := m.Initialize(ctx, world); err != nil {
		return nil, err
	}
	s := &Simulation{
		cfg:   cfg,
		model: m,
		world: world,
		exec:  exec,
		state: StateIdle,
	}
	s.cond = sync.NewCond(&s.mu)
	return s, nil
}

// World returns the simulation's world. It is safe to inspect while the
// simulation is paused or stopped.
func (s *Simulation) World() *World { return s.world }

// State returns the current lifecycle state.
func (s *Simulation) State() State {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.state
}

// Err returns the error that caused a failed run, or nil.
func (s *Simulation) Err() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.err
}

// Ticks returns the number of completed steps (ticks for tick-mode models). It
// counts steps within the current run session; it is reset at the start of each
// Run/RunN/Start and is not cumulative across runs.
func (s *Simulation) Ticks() uint64 { return s.steps.Load() }

// Config returns the simulation configuration.
func (s *Simulation) Config() Config { return s.cfg }

// SetStepObserver registers a callback invoked after each step with the step
// duration. Used for tick-duration metrics. It is not part of determinism.
func (s *Simulation) SetStepObserver(fn func(time.Duration)) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.stepObserver = fn
}

// Snapshot captures the current simulation state, including the model's own
// configuration (when the model implements SnapshotModel). It returns an error
// if the simulation is currently running, so a snapshot is never taken mid-tick
// (which would otherwise be torn across the world's internal structures).
func (s *Simulation) Snapshot() (*Snapshot, error) {
	s.mu.Lock()
	if s.state == StateRunning {
		s.mu.Unlock()
		return nil, ErrAlreadyRunning
	}
	s.mu.Unlock()

	snap, err := s.world.Snapshot()
	if err != nil {
		return nil, err
	}
	if m, ok := s.model.(SnapshotModel); ok {
		cfg := m.SnapshotConfig()
		if cfg != nil {
			raw, err := json.Marshal(cfg)
			if err != nil {
				return nil, fmt.Errorf("simulation: snapshot model config: %w", err)
			}
			snap.ModelConfig = raw
			checksum, err := snap.computeChecksum()
			if err != nil {
				return nil, err
			}
			snap.Checksum = checksum
		}
	}
	return snap, nil
}

// Restore overwrites the simulation state from a snapshot, including the
// model's configuration when present and the model implements SnapshotModel.
// It returns an error if the simulation is currently running.
func (s *Simulation) Restore(snap *Snapshot) error {
	s.mu.Lock()
	if s.state == StateRunning {
		s.mu.Unlock()
		return ErrAlreadyRunning
	}
	s.mu.Unlock()

	if err := s.world.Restore(snap); err != nil {
		return err
	}
	if m, ok := s.model.(SnapshotModel); ok && len(snap.ModelConfig) > 0 {
		if err := m.RestoreConfig(snap.ModelConfig); err != nil {
			return fmt.Errorf("simulation: restore model config: %w", err)
		}
	}
	return nil
}

// Run executes the simulation to completion and blocks until it finishes.
// It returns the run error, if any.
func (s *Simulation) Run(ctx context.Context) error {
	done, err := s.begin()
	if err != nil {
		return err
	}
	return s.run(ctx, s.defaultLimit(), done)
}

// RunN executes exactly n steps synchronously, then stops.
func (s *Simulation) RunN(ctx context.Context, n uint64) error {
	done, err := s.begin()
	if err != nil {
		return err
	}
	limit := func() bool { return s.steps.Load() >= n }
	return s.run(ctx, limit, done)
}

// RunUntil executes synchronously until pred returns true. pred is called once
// after each step; it must not call methods on the Simulation.
func (s *Simulation) RunUntil(ctx context.Context, pred func() bool) error {
	done, err := s.begin()
	if err != nil {
		return err
	}
	return s.run(ctx, pred, done)
}

// Start launches the simulation in the background and returns immediately.
// Completion is observed via State or Wait.
func (s *Simulation) Start(ctx context.Context) error {
	done, err := s.begin()
	if err != nil {
		return err
	}
	go s.run(ctx, s.defaultLimit(), done)
	return nil
}

// Wait blocks until the current run finishes and returns its error. It returns
// nil if no run has ever started.
func (s *Simulation) Wait() error {
	s.mu.Lock()
	done := s.done
	s.mu.Unlock()
	if done == nil {
		return nil
	}
	<-done
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.err
}

// Step performs a single unit of work (one tick, or one event) synchronously.
// It returns an error if the simulation is currently running in the
// background. On error the simulation transitions to StateFailed.
func (s *Simulation) Step(ctx context.Context) error {
	s.mu.Lock()
	if s.state == StateRunning {
		s.mu.Unlock()
		return ErrAlreadyRunning
	}
	s.mu.Unlock()

	start := time.Now()
	_, err := s.exec.step(ctx, s.world)
	s.mu.Lock()
	if obs := s.stepObserver; obs != nil {
		obs(time.Since(start))
	}
	if err != nil {
		s.state = StateFailed
		s.err = err
	} else {
		s.steps.Add(1)
	}
	s.mu.Unlock()
	return err
}

// Pause suspends a running simulation. It takes effect at the next step
// boundary. Returns ErrNotRunning if the simulation is not running.
func (s *Simulation) Pause() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.state != StateRunning {
		return ErrNotRunning
	}
	s.state = StatePaused
	return nil
}

// Resume continues a paused simulation. Returns ErrNotRunning if not paused.
func (s *Simulation) Resume() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.state != StatePaused {
		return ErrNotRunning
	}
	s.state = StateRunning
	s.cond.Broadcast()
	return nil
}

// Stop halts a running or paused simulation. Returns ErrNotRunning if the
// simulation is not running.
func (s *Simulation) Stop() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.state != StateRunning && s.state != StatePaused {
		return ErrNotRunning
	}
	s.state = StateStopped
	if s.cancel != nil {
		s.cancel()
	}
	s.cond.Broadcast()
	return nil
}

// begin transitions the simulation into StateRunning and returns a fresh done
// channel, or an error if it is already running.
func (s *Simulation) begin() (chan struct{}, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.state == StateRunning {
		return nil, ErrAlreadyRunning
	}
	s.state = StateRunning
	s.err = nil
	s.steps.Store(0)
	done := make(chan struct{})
	s.done = done
	return done, nil
}

// defaultLimit builds the stop condition from Config.
func (s *Simulation) defaultLimit() func() bool {
	switch s.cfg.Mode {
	case model.ModeTick:
		if s.cfg.MaxTicks > 0 {
			return func() bool { return s.steps.Load() >= s.cfg.MaxTicks }
		}
		return nil
	case model.ModeEvent:
		if s.cfg.MaxTime > 0 {
			return func() bool {
				e, ok := s.world.Events.Peek()
				if !ok {
					return true
				}
				return e.Time >= s.cfg.MaxTime
			}
		}
		return nil
	default:
		return nil
	}
}

// run is the shared execution loop. limit, if non-nil, is consulted before each
// step. The run terminates on error, on limit, on Stop/cancellation, or (in
// event mode) when no events remain. Deferred finalization guarantees cleanup
// (cancel + close(done)) even if the model panics, and converts a panic into a
// failed state so Wait() always returns.
func (s *Simulation) run(ctx context.Context, limit func() bool, done chan struct{}) (runErr error) {
	runCtx, cancel := context.WithCancel(ctx)
	s.mu.Lock()
	s.cancel = cancel
	stepObserver := s.stepObserver
	s.mu.Unlock()

	defer func() {
		if r := recover(); r != nil {
			runErr = fmt.Errorf("simulation: model panic: %v", r)
		}
		s.mu.Lock()
		if runErr != nil {
			s.state = StateFailed
			s.err = runErr
			s.cond.Broadcast()
		} else if s.state == StateRunning {
			s.state = StateCompleted
		}
		s.cancel = nil
		s.mu.Unlock()
		cancel()
		close(done)
	}()

	// Watcher: propagate cancellation of the run context into a clean stop so
	// a paused loop also wakes up.
	go func() {
		<-runCtx.Done()
		s.mu.Lock()
		if s.state == StateRunning || s.state == StatePaused {
			s.state = StateStopped
			s.cond.Broadcast()
		}
		s.mu.Unlock()
	}()

	for {
		s.mu.Lock()
		for s.state == StatePaused {
			s.cond.Wait()
		}
		if s.state != StateRunning {
			s.mu.Unlock()
			break
		}
		s.mu.Unlock()

		if limit != nil && limit() {
			s.mu.Lock()
			if s.state == StateRunning {
				s.state = StateCompleted
			}
			s.mu.Unlock()
			break
		}

		start := time.Now()
		more, err := s.exec.step(runCtx, s.world)
		if stepObserver != nil {
			stepObserver(time.Since(start))
		}
		if err != nil {
			if runCtx.Err() != nil {
				// The run context was cancelled (Stop or external cancellation)
				// while a step was in flight; the scheduler surfaced the
				// cancellation as an error. This is a stop, not a model failure.
				s.mu.Lock()
				if s.state == StateRunning || s.state == StatePaused {
					s.state = StateStopped
					s.cond.Broadcast()
				}
				s.mu.Unlock()
				break
			}
			runErr = err
			s.mu.Lock()
			s.state = StateFailed
			s.err = err
			s.cond.Broadcast()
			s.mu.Unlock()
			break
		}

		if !more {
			s.mu.Lock()
			if s.state == StateRunning {
				s.state = StateCompleted
			}
			s.mu.Unlock()
			break
		}

		s.steps.Add(1)
	}
	return
}
