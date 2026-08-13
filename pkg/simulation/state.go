package simulation

import "errors"

// State is the lifecycle state of a Simulation.
type State uint32

const (
	// StateIdle is the initial state after construction, before first run.
	StateIdle State = iota
	// StateRunning means the simulation is actively executing.
	StateRunning
	// StatePaused means execution is temporarily suspended.
	StatePaused
	// StateStopped means execution was halted (cancellation or explicit stop).
	StateStopped
	// StateCompleted means execution finished normally (limit reached).
	StateCompleted
	// StateFailed means execution terminated with an error.
	StateFailed
)

func (s State) String() string {
	switch s {
	case StateIdle:
		return "idle"
	case StateRunning:
		return "running"
	case StatePaused:
		return "paused"
	case StateStopped:
		return "stopped"
	case StateCompleted:
		return "completed"
	case StateFailed:
		return "failed"
	default:
		return "unknown"
	}
}

var (
	// ErrAlreadyRunning is returned when Run is called on a running simulation.
	ErrAlreadyRunning = errors.New("simulation: already running")
	// ErrNotRunning is returned when Pause/Resume/Stop is called on a simulation
	// that is not running.
	ErrNotRunning = errors.New("simulation: not running")
	// ErrBadMode is returned when the config mode does not match the model.
	ErrBadMode = errors.New("simulation: model does not support configured mode")
)
