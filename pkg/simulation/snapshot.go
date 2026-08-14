package simulation

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"

	"github.com/ripper19/simulator/pkg/model"
)

// EngineVersion is the semantic version of the simulation engine. It is
// recorded in every snapshot so a snapshot can be validated against the engine
// that produced it.
const EngineVersion = "0.1.0"

// SnapshotSchemaVersion is the version of the snapshot format. It is bumped
// whenever the serialized schema changes incompatibly.
const SnapshotSchemaVersion = 1

// componentSnapshot is the serialized form of one component column: the
// component name, the entities holding it (in dense order), and the JSON-encoded
// values aligned with those entities.
type componentSnapshot struct {
	Name     string          `json:"name"`
	Entities []EntityID      `json:"entities"`
	Values   json.RawMessage `json:"values"`
}

// Snapshot is a complete, versioned, self-validating capture of a simulation's
// state: provenance (IDs, model, seed, mode), clock, entity allocation state,
// all component columns, and the event queue. A snapshot contains enough
// information to restore execution deterministically.
type Snapshot struct {
	SchemaVersion int    `json:"schema_version"`
	EngineVersion string `json:"engine_version"`
	Checksum      string `json:"checksum,omitempty"`

	SimulationID string `json:"simulation_id"`
	ModelID      string `json:"model_id"`
	ModelVersion string `json:"model_version"`
	Seed         uint64 `json:"seed"`
	Mode         string `json:"mode"`

	Tick uint64  `json:"tick"`
	Time float64 `json:"time"`

	Entities   entityManagerState  `json:"entities"`
	Components []componentSnapshot `json:"components"`
	Events     eventQueueState     `json:"events"`
}

// computeChecksum hashes the canonical JSON encoding of the snapshot (with the
// checksum field cleared), yielding a SHA-256 hex digest for integrity checks.
func (s *Snapshot) computeChecksum() (string, error) {
	clone := *s
	clone.Checksum = ""
	b, err := json.Marshal(&clone)
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256(b)
	return hex.EncodeToString(sum[:]), nil
}

// Validate checks the snapshot's schema version, engine version, and checksum.
// It returns a non-nil error if the snapshot is incompatible or corrupted.
func (s *Snapshot) Validate() error {
	if s.SchemaVersion != SnapshotSchemaVersion {
		return fmt.Errorf("simulation: snapshot schema version %d unsupported (want %d)", s.SchemaVersion, SnapshotSchemaVersion)
	}
	if s.EngineVersion != EngineVersion {
		return fmt.Errorf("simulation: snapshot engine version %q unsupported (want %q)", s.EngineVersion, EngineVersion)
	}
	want, err := s.computeChecksum()
	if err != nil {
		return err
	}
	if s.Checksum != want {
		return fmt.Errorf("simulation: snapshot checksum mismatch (corrupted)")
	}
	return nil
}

// Snapshot captures the current world state. The result is deterministic: two
// worlds in an identical state produce identical snapshots.
func (w *World) Snapshot() (*Snapshot, error) {
	components, err := w.Components.Snapshot()
	if err != nil {
		return nil, err
	}
	events, err := w.Events.snapshot()
	if err != nil {
		return nil, err
	}
	s := &Snapshot{
		SchemaVersion: SnapshotSchemaVersion,
		EngineVersion: EngineVersion,
		SimulationID:  w.Meta.SimulationID,
		ModelID:       w.Meta.ModelID,
		ModelVersion:  w.Meta.ModelVersion,
		Seed:          w.Meta.Seed,
		Mode:          w.Meta.Mode.String(),
		Tick:          w.Clock.Tick(),
		Time:          w.Clock.Time(),
		Entities:      w.Entities.snapshot(),
		Components:    components,
		Events:        events,
	}
	checksum, err := s.computeChecksum()
	if err != nil {
		return nil, err
	}
	s.Checksum = checksum
	return s, nil
}

// Restore overwrites the world state with the snapshot's state. It validates
// the snapshot and requires that it was produced by the same model, version,
// and seed as this world (restoring across model versions or seeds is rejected
// to preserve determinism). The caller must not mutate the world concurrently.
func (w *World) Restore(s *Snapshot) error {
	if err := s.Validate(); err != nil {
		return err
	}
	if s.ModelID != w.Meta.ModelID || s.ModelVersion != w.Meta.ModelVersion {
		return fmt.Errorf("simulation: snapshot model %s@%s does not match world model %s@%s",
			s.ModelID, s.ModelVersion, w.Meta.ModelID, w.Meta.ModelVersion)
	}
	if s.Seed != w.Meta.Seed {
		return fmt.Errorf("simulation: snapshot seed %d does not match world seed %d", s.Seed, w.Meta.Seed)
	}

	w.Entities.restore(s.Entities)
	if err := w.Components.Restore(s.Components); err != nil {
		return err
	}
	w.Clock.Set(s.Tick, s.Time)
	w.Events.restore(s.Events)
	w.Meta.SimulationID = s.SimulationID
	w.Meta.Mode = modeFromString(s.Mode)
	return nil
}

func modeFromString(s string) model.Mode {
	switch s {
	case "tick":
		return model.ModeTick
	case "event":
		return model.ModeEvent
	default:
		return model.ModeTick
	}
}
