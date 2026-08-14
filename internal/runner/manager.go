// Package runner provides the in-process simulation manager: it creates,
// executes, and controls simulations and persists their records and snapshots
// through the persistence store. This is the "local worker" execution path; the
// distributed worker layer (a later phase) dispatches the same models to
// remote workers without model changes.
package runner

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"sync"
	"time"

	"github.com/ripper19/simulator/internal/metrics"
	"github.com/ripper19/simulator/internal/persistence"
	"github.com/ripper19/simulator/internal/registry"
	"github.com/ripper19/simulator/pkg/model"
	"github.com/ripper19/simulator/pkg/simulation"
)

// ErrNotFound is returned when a simulation does not exist.
var ErrNotFound = errors.New("runner: simulation not found")

// CreateRequest is the payload for creating a simulation.
type CreateRequest struct {
	ID           string          `json:"id,omitempty"`
	ModelID      string          `json:"model_id"`
	ModelVersion string          `json:"model_version,omitempty"`
	Seed         uint64          `json:"seed"`
	Mode         string          `json:"mode,omitempty"`
	MaxTicks     uint64          `json:"max_ticks,omitempty"`
	MaxTime      float64         `json:"max_time,omitempty"`
	Workers      int             `json:"workers,omitempty"`
	Config       json.RawMessage `json:"config,omitempty"`
	OwnerID      string          `json:"owner_id,omitempty"`
}

// State is the runtime state of a simulation.
type State struct {
	ID     string  `json:"id"`
	Status string  `json:"status"`
	Tick   uint64  `json:"tick"`
	Time   float64 `json:"time"`
	Steps  uint64  `json:"steps"`
}

// Metrics is a lightweight runtime metrics snapshot.
type Metrics struct {
	ID       string  `json:"id"`
	Status   string  `json:"status"`
	Steps    uint64  `json:"steps"`
	Tick     uint64  `json:"tick"`
	Time     float64 `json:"time"`
	Entities int     `json:"entities"`
	Events   int     `json:"events"`
}

// Manager holds and controls in-process simulations.
type Manager struct {
	mu       sync.RWMutex
	sims     map[string]*managed
	store    *persistence.Store
	registry *registry.Registry
	met      *metrics.Metrics
}

type managed struct {
	sim   *simulation.Simulation
	model simulation.Model
	info  persistence.SimulationInfo
}

// NewManager returns a Manager backed by the store and model registry.
func NewManager(store *persistence.Store, reg *registry.Registry) *Manager {
	return &Manager{sims: make(map[string]*managed), store: store, registry: reg}
}

// SetMetrics attaches a metrics set (may be nil to disable).
func (m *Manager) SetMetrics(met *metrics.Metrics) {
	m.met = met
}

func (m *Manager) observe(sim *simulation.Simulation) {
	if m.met == nil {
		return
	}
	sim.SetStepObserver(func(d time.Duration) { m.met.TickDuration.Observe(d.Seconds()) })
}

// runConfig is the full run configuration, persisted in the simulation record's
// config column so a simulation can be replayed deterministically from its seed.
type runConfig struct {
	ModelID      string          `json:"model_id"`
	ModelVersion string          `json:"model_version"`
	Seed         uint64          `json:"seed"`
	Mode         string          `json:"mode"`
	MaxTicks     uint64          `json:"max_ticks"`
	MaxTime      float64         `json:"max_time"`
	Workers      int             `json:"workers"`
	Model        json.RawMessage `json:"model"`
}

// Create instantiates and registers a new simulation.
func (m *Manager) Create(ctx context.Context, req CreateRequest) (persistence.SimulationInfo, error) {
	mg, err := m.instantiate(ctx, req)
	if err != nil {
		return persistence.SimulationInfo{}, err
	}
	info, err := m.store.CreateSimulation(ctx, mg.info)
	if err != nil {
		return persistence.SimulationInfo{}, fmt.Errorf("runner: persist simulation: %w", err)
	}
	mg.info = info
	m.mu.Lock()
	m.sims[info.ID] = mg
	m.mu.Unlock()
	return info, nil
}

// instantiate builds a simulation and its record from a create request without
// persisting it.
func (m *Manager) instantiate(ctx context.Context, req CreateRequest) (*managed, error) {
	if req.Seed > math.MaxInt64 {
		return nil, fmt.Errorf("runner: seed %d exceeds int64 range (PostgreSQL BIGINT); use a seed < 2^63", req.Seed)
	}
	entry, ok := m.registry.Get(req.ModelID)
	if !ok {
		return nil, fmt.Errorf("runner: unknown model %q", req.ModelID)
	}
	if req.ModelVersion != "" {
		if v, ok := m.registry.GetVersion(req.ModelID, req.ModelVersion); ok {
			entry = v
		} else {
			return nil, fmt.Errorf("runner: unknown model version %s@%s", req.ModelID, req.ModelVersion)
		}
	}
	if entry.Factory == nil {
		return nil, fmt.Errorf("runner: model %s@%s has no factory", entry.Info.ID, entry.Info.Version)
	}
	// Ensure the model's metadata is persisted so the simulation's foreign key
	// to the model registry is satisfied.
	if err := m.store.UpsertModel(ctx, entry.Info); err != nil {
		return nil, fmt.Errorf("runner: persist model: %w", err)
	}

	mdl := entry.Factory()
	if len(req.Config) > 0 {
		if cm, ok := mdl.(simulation.ConfigurableModel); ok {
			if err := cm.Configure(req.Config); err != nil {
				return nil, fmt.Errorf("runner: configure model: %w", err)
			}
		}
	}

	mode := mdl.Metadata().Mode
	if req.Mode != "" {
		switch req.Mode {
		case "tick":
			mode = model.ModeTick
		case "event":
			mode = model.ModeEvent
		default:
			return nil, fmt.Errorf("runner: invalid mode %q", req.Mode)
		}
	}

	id := req.ID
	if id == "" {
		id = newID()
	}

	cfg := simulation.Config{
		ID:       id,
		Seed:     req.Seed,
		Mode:     mode,
		MaxTicks: req.MaxTicks,
		MaxTime:  req.MaxTime,
		Workers:  req.Workers,
	}
	sim, err := simulation.New(ctx, cfg, mdl)
	if err != nil {
		return nil, fmt.Errorf("runner: init simulation: %w", err)
	}
	m.observe(sim)

	runCfg, err := json.Marshal(runConfig{
		ModelID:      entry.Info.ID,
		ModelVersion: entry.Info.Version,
		Seed:         req.Seed,
		Mode:         mode.String(),
		MaxTicks:     req.MaxTicks,
		MaxTime:      req.MaxTime,
		Workers:      req.Workers,
		Model:        req.Config,
	})
	if err != nil {
		return nil, err
	}

	return &managed{
		sim:   sim,
		model: mdl,
		info: persistence.SimulationInfo{
			ID:           id,
			ModelID:      entry.Info.ID,
			ModelVersion: entry.Info.Version,
			Seed:         int64(req.Seed),
			Mode:         mode.String(),
			Status:       "created",
			Config:       runCfg,
			OwnerID:      ownerPtr(req.OwnerID),
		},
	}, nil
}

func ownerPtr(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}

// Replay deterministically re-runs a simulation from its seed and stored
// configuration. It is asynchronous (like Start): it rebuilds the simulation
// from its seed and starts it in the background, returning immediately.
func (m *Manager) Replay(ctx context.Context, id string) error {
	m.mu.RLock()
	mg, ok := m.sims[id]
	m.mu.RUnlock()
	if !ok {
		return ErrNotFound
	}
	if err := mg.sim.Stop(); err != nil && !errors.Is(err, simulation.ErrNotRunning) {
		return err
	}

	var rc runConfig
	if err := json.Unmarshal(mg.info.Config, &rc); err != nil {
		return fmt.Errorf("runner: parse run config: %w", err)
	}

	fresh, err := m.instantiate(ctx, CreateRequest{
		ID:           id,
		ModelID:      rc.ModelID,
		ModelVersion: rc.ModelVersion,
		Seed:         rc.Seed,
		Mode:         rc.Mode,
		MaxTicks:     rc.MaxTicks,
		MaxTime:      rc.MaxTime,
		Workers:      rc.Workers,
		Config:       rc.Model,
	})
	if err != nil {
		return err
	}

	m.mu.Lock()
	m.sims[id] = fresh
	m.mu.Unlock()

	if err := fresh.sim.Start(context.Background()); err != nil {
		return err
	}
	m.updateStatus(id, "running")
	go m.watch(id)
	return nil
}

// Get returns a simulation's record with the current (live) status.
func (m *Manager) Get(id string) (persistence.SimulationInfo, bool) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	mg, ok := m.sims[id]
	if !ok {
		return persistence.SimulationInfo{}, false
	}
	info := mg.info
	info.Status = statusOf(mg.sim.State())
	return info, true
}

// List returns all simulation records with their current (live) status.
func (m *Manager) List() []persistence.SimulationInfo {
	m.mu.RLock()
	defer m.mu.RUnlock()
	out := make([]persistence.SimulationInfo, 0, len(m.sims))
	for _, mg := range m.sims {
		info := mg.info
		info.Status = statusOf(mg.sim.State())
		out = append(out, info)
	}
	return out
}

// Delete removes a simulation. It refuses to delete a running simulation.
func (m *Manager) Delete(ctx context.Context, id string) error {
	m.mu.RLock()
	mg, ok := m.sims[id]
	m.mu.RUnlock()
	if !ok {
		return ErrNotFound
	}
	if mg.sim.State() == simulation.StateRunning || mg.sim.State() == simulation.StatePaused {
		return errors.New("runner: cannot delete a running simulation")
	}
	if err := mg.sim.Stop(); err != nil && !errors.Is(err, simulation.ErrNotRunning) {
		return err
	}
	m.mu.Lock()
	delete(m.sims, id)
	m.mu.Unlock()
	if err := m.store.DeleteSimulation(ctx, id); err != nil {
		return fmt.Errorf("runner: delete simulation: %w", err)
	}
	return nil
}

// Start begins executing a simulation in the background. The simulation runs on
// a background context independent of any HTTP request, so it continues after
// the start request returns.
func (m *Manager) Start(ctx context.Context, id string) error {
	mg, err := m.lookup(id)
	if err != nil {
		return err
	}
	if err := mg.sim.Start(context.Background()); err != nil {
		return err
	}
	if m.met != nil {
		m.met.SimulationStarted.Inc()
		m.met.SimulationActive.Inc()
	}
	m.updateStatus(id, "running")
	go m.watch(id)
	return nil
}

// Pause suspends a running simulation.
func (m *Manager) Pause(ctx context.Context, id string) error {
	return m.control(id, "paused", (*simulation.Simulation).Pause)
}

// Resume continues a paused simulation.
func (m *Manager) Resume(ctx context.Context, id string) error {
	return m.control(id, "running", (*simulation.Simulation).Resume)
}

// Stop halts a simulation.
func (m *Manager) Stop(ctx context.Context, id string) error {
	return m.control(id, "stopped", (*simulation.Simulation).Stop)
}

// control applies a simulation operation and records the resulting status.
func (m *Manager) control(id, status string, f func(*simulation.Simulation) error) error {
	mg, err := m.lookup(id)
	if err != nil {
		return err
	}
	if err := f(mg.sim); err != nil {
		return err
	}
	m.updateStatus(id, status)
	return nil
}

// Step advances a simulation by one unit of work.
func (m *Manager) Step(ctx context.Context, id string) error {
	mg, err := m.lookup(id)
	if err != nil {
		return err
	}
	if err := mg.sim.Step(ctx); err != nil {
		return err
	}
	m.updateStatus(id, statusOf(mg.sim.State()))
	return nil
}

// Snapshot captures and persists a simulation snapshot.
func (m *Manager) Snapshot(ctx context.Context, id string) (*simulation.Snapshot, error) {
	mg, err := m.lookup(id)
	if err != nil {
		return nil, err
	}
	snap, err := mg.sim.Snapshot()
	if err != nil {
		return nil, err
	}
	data, err := json.Marshal(snap)
	if err != nil {
		return nil, err
	}
	if m.met != nil {
		m.met.SnapshotSize.Observe(float64(len(data)))
	}
	err = m.store.SaveSnapshot(ctx, persistence.SnapshotInfo{
		ID:            newID(),
		SimulationID:  id,
		SchemaVersion: int32(snap.SchemaVersion),
		EngineVersion: snap.EngineVersion,
		Data:          data,
		Checksum:      snap.Checksum,
	})
	if err != nil {
		return nil, err
	}
	return snap, nil
}

// Restore overwrites a simulation's state from a snapshot.
func (m *Manager) Restore(ctx context.Context, id string, snap *simulation.Snapshot) error {
	mg, err := m.lookup(id)
	if err != nil {
		return err
	}
	return mg.sim.Restore(snap)
}

// State returns a simulation's runtime state.
func (m *Manager) State(ctx context.Context, id string) (State, error) {
	mg, err := m.lookup(id)
	if err != nil {
		return State{}, err
	}
	s := mg.sim
	return State{
		ID:     id,
		Status: statusOf(s.State()),
		Tick:   s.World().Clock.Tick(),
		Time:   s.World().Clock.Time(),
		Steps:  s.Ticks(),
	}, nil
}

// Metrics returns a simulation's runtime metrics.
func (m *Manager) Metrics(ctx context.Context, id string) (Metrics, error) {
	mg, err := m.lookup(id)
	if err != nil {
		return Metrics{}, err
	}
	s := mg.sim
	return Metrics{
		ID:       id,
		Status:   statusOf(s.State()),
		Steps:    s.Ticks(),
		Tick:     s.World().Clock.Tick(),
		Time:     s.World().Clock.Time(),
		Entities: s.World().Entities.Len(),
		Events:   s.World().Events.Len(),
	}, nil
}

// Simulation exposes the underlying simulation for read-only access (state,
// events). Callers must not mutate it concurrently with a running simulation.
func (m *Manager) Simulation(id string) (*simulation.Simulation, bool) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	mg, ok := m.sims[id]
	if !ok {
		return nil, false
	}
	return mg.sim, true
}

func (m *Manager) lookup(id string) (*managed, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	mg, ok := m.sims[id]
	if !ok {
		return nil, ErrNotFound
	}
	return mg, nil
}

func (m *Manager) updateStatus(id, status string) {
	if _, err := m.store.UpdateSimulationStatus(context.Background(), id, status); err != nil {
		return
	}
}

// watch waits for a simulation to finish and updates its persisted status and
// metrics.
func (m *Manager) watch(id string) {
	mg, err := m.lookup(id)
	if err != nil {
		return
	}
	if err := mg.sim.Wait(); err != nil {
		_ = err // status is derived from state below
	}
	if m.met != nil {
		m.met.SimulationActive.Dec()
		if mg.sim.State() == simulation.StateFailed {
			m.met.SimulationFailed.Inc()
		} else {
			m.met.SimulationCompleted.Inc()
		}
	}
	m.updateStatus(id, statusOf(mg.sim.State()))
}

func statusOf(s simulation.State) string {
	switch s {
	case simulation.StateRunning:
		return "running"
	case simulation.StatePaused:
		return "paused"
	case simulation.StateCompleted:
		return "completed"
	case simulation.StateFailed:
		return "failed"
	case simulation.StateStopped:
		return "stopped"
	default:
		return "created"
	}
}

func newID() string {
	var b [8]byte
	if _, err := rand.Read(b[:]); err != nil {
		return fmt.Sprintf("%d", len(b))
	}
	return hex.EncodeToString(b[:])
}
