// Package persistence provides the PostgreSQL-backed repository for the
// platform's durable state: the model registry, simulations, and snapshots.
// It wraps sqlc-generated queries with clean domain types.
package persistence

import (
	"context"
	"encoding/json"
	"time"

	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/ripper19/simulator/internal/persistence/sqlc"
)

// ModelInfo is a durable model-registry entry.
type ModelInfo struct {
	ID            string          `json:"id"`
	Name          string          `json:"name"`
	Version       string          `json:"version"`
	Description   string          `json:"description"`
	Mode          string          `json:"mode"`
	ConfigSchema  json.RawMessage `json:"config_schema,omitempty"`
	RuntimeCompat string          `json:"runtime_compat,omitempty"`
	Author        string          `json:"author,omitempty"`
	CreatedAt     time.Time       `json:"created_at"`
}

// SimulationInfo is a durable simulation record.
type SimulationInfo struct {
	ID           string          `json:"id"`
	ModelID      string          `json:"model_id"`
	ModelVersion string          `json:"model_version"`
	Seed         int64           `json:"seed"`
	Mode         string          `json:"mode"`
	Status       string          `json:"status"`
	Config       json.RawMessage `json:"config,omitempty"`
	CreatedAt    time.Time       `json:"created_at"`
	UpdatedAt    time.Time       `json:"updated_at"`
	CompletedAt  *time.Time      `json:"completed_at,omitempty"`
}

// SnapshotInfo is a durable snapshot record.
type SnapshotInfo struct {
	ID            string          `json:"id"`
	SimulationID  string          `json:"simulation_id"`
	SchemaVersion int32           `json:"schema_version"`
	EngineVersion string          `json:"engine_version"`
	Data          json.RawMessage `json:"data"`
	Checksum      string          `json:"checksum"`
	CreatedAt     time.Time       `json:"created_at"`
}

// Store is the persistence layer, backed by a pgx connection pool and the
// sqlc-generated queries.
type Store struct {
	pool *pgxpool.Pool
	q    *sqlc.Queries
}

// NewStore returns a Store over the given pool.
func NewStore(pool *pgxpool.Pool) *Store {
	return &Store{pool: pool, q: sqlc.New(pool)}
}

// Pool exposes the underlying pool for transaction control by callers.
func (s *Store) Pool() *pgxpool.Pool { return s.pool }

// ---- models ----

// UpsertModel inserts or updates a model-registry entry.
func (s *Store) UpsertModel(ctx context.Context, m ModelInfo) error {
	cs := m.ConfigSchema
	if len(cs) == 0 {
		cs = json.RawMessage(`{}`)
	}
	return s.q.UpsertModel(ctx, sqlc.UpsertModelParams{
		ID:            m.ID,
		Name:          m.Name,
		Version:       m.Version,
		Description:   m.Description,
		Mode:          m.Mode,
		ConfigSchema:  cs,
		RuntimeCompat: m.RuntimeCompat,
		Author:        m.Author,
	})
}

// ListModels returns all registered models.
func (s *Store) ListModels(ctx context.Context) ([]ModelInfo, error) {
	rows, err := s.q.ListModels(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]ModelInfo, 0, len(rows))
	for _, r := range rows {
		out = append(out, modelFromRow(r))
	}
	return out, nil
}

// GetModel returns a specific model version.
func (s *Store) GetModel(ctx context.Context, id, version string) (ModelInfo, error) {
	r, err := s.q.GetModel(ctx, sqlc.GetModelParams{ID: id, Version: version})
	if err != nil {
		return ModelInfo{}, err
	}
	return modelFromRow(r), nil
}

// ListModelVersions returns all versions of a model.
func (s *Store) ListModelVersions(ctx context.Context, id string) ([]ModelInfo, error) {
	rows, err := s.q.ListModelVersions(ctx, id)
	if err != nil {
		return nil, err
	}
	out := make([]ModelInfo, 0, len(rows))
	for _, r := range rows {
		out = append(out, modelFromRow(r))
	}
	return out, nil
}

// ---- simulations ----

// CreateSimulation persists a new simulation record and returns it.
func (s *Store) CreateSimulation(ctx context.Context, si SimulationInfo) (SimulationInfo, error) {
	cfg := si.Config
	if len(cfg) == 0 {
		cfg = json.RawMessage(`{}`)
	}
	r, err := s.q.CreateSimulation(ctx, sqlc.CreateSimulationParams{
		ID:           si.ID,
		ModelID:      si.ModelID,
		ModelVersion: si.ModelVersion,
		Seed:         si.Seed,
		Mode:         si.Mode,
		Status:       si.Status,
		Config:       cfg,
	})
	if err != nil {
		return SimulationInfo{}, err
	}
	return simulationFromRow(r), nil
}

// GetSimulation returns a simulation by ID.
func (s *Store) GetSimulation(ctx context.Context, id string) (SimulationInfo, error) {
	r, err := s.q.GetSimulation(ctx, id)
	if err != nil {
		return SimulationInfo{}, err
	}
	return simulationFromRow(r), nil
}

// ListSimulations returns all simulations, newest first.
func (s *Store) ListSimulations(ctx context.Context) ([]SimulationInfo, error) {
	rows, err := s.q.ListSimulations(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]SimulationInfo, 0, len(rows))
	for _, r := range rows {
		out = append(out, simulationFromRow(r))
	}
	return out, nil
}

// UpdateSimulationStatus updates a simulation's status and returns the updated
// record. Terminal statuses set completed_at.
func (s *Store) UpdateSimulationStatus(ctx context.Context, id, status string) (SimulationInfo, error) {
	r, err := s.q.UpdateSimulationStatus(ctx, sqlc.UpdateSimulationStatusParams{
		ID:     id,
		Status: status,
	})
	if err != nil {
		return SimulationInfo{}, err
	}
	return simulationFromRow(r), nil
}

// ---- snapshots ----

// SaveSnapshot persists a snapshot.
func (s *Store) SaveSnapshot(ctx context.Context, sn SnapshotInfo) error {
	return s.q.SaveSnapshot(ctx, sqlc.SaveSnapshotParams{
		ID:            sn.ID,
		SimulationID:  sn.SimulationID,
		SchemaVersion: sn.SchemaVersion,
		EngineVersion: sn.EngineVersion,
		Data:          sn.Data,
		Checksum:      sn.Checksum,
	})
}

// GetSnapshot returns a snapshot by ID.
func (s *Store) GetSnapshot(ctx context.Context, id string) (SnapshotInfo, error) {
	r, err := s.q.GetSnapshot(ctx, id)
	if err != nil {
		return SnapshotInfo{}, err
	}
	return snapshotFromRow(r), nil
}

// ListSnapshots returns snapshots for a simulation, newest first.
func (s *Store) ListSnapshots(ctx context.Context, simulationID string) ([]SnapshotInfo, error) {
	rows, err := s.q.ListSnapshots(ctx, simulationID)
	if err != nil {
		return nil, err
	}
	out := make([]SnapshotInfo, 0, len(rows))
	for _, r := range rows {
		out = append(out, snapshotFromRow(r))
	}
	return out, nil
}

// ---- conversions ----

func modelFromRow(r sqlc.Model) ModelInfo {
	return ModelInfo{
		ID:            r.ID,
		Name:          r.Name,
		Version:       r.Version,
		Description:   r.Description,
		Mode:          r.Mode,
		ConfigSchema:  r.ConfigSchema,
		RuntimeCompat: r.RuntimeCompat,
		Author:        r.Author,
		CreatedAt:     tsTime(r.CreatedAt),
	}
}

func simulationFromRow(r sqlc.Simulation) SimulationInfo {
	return SimulationInfo{
		ID:           r.ID,
		ModelID:      r.ModelID,
		ModelVersion: r.ModelVersion,
		Seed:         r.Seed,
		Mode:         r.Mode,
		Status:       r.Status,
		Config:       r.Config,
		CreatedAt:    tsTime(r.CreatedAt),
		UpdatedAt:    tsTime(r.UpdatedAt),
		CompletedAt:  tsTimePtr(r.CompletedAt),
	}
}

func snapshotFromRow(r sqlc.Snapshot) SnapshotInfo {
	return SnapshotInfo{
		ID:            r.ID,
		SimulationID:  r.SimulationID,
		SchemaVersion: r.SchemaVersion,
		EngineVersion: r.EngineVersion,
		Data:          r.Data,
		Checksum:      r.Checksum,
		CreatedAt:     tsTime(r.CreatedAt),
	}
}

func tsTime(t pgtype.Timestamptz) time.Time {
	if !t.Valid {
		return time.Time{}
	}
	return t.Time
}

func tsTimePtr(t pgtype.Timestamptz) *time.Time {
	if !t.Valid {
		return nil
	}
	tt := t.Time
	return &tt
}
