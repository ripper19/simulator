// Package counter implements CounterWorld, a deliberately trivial test model
// used to exercise the engine's determinism, parallelism, snapshots, replay,
// and performance characteristics.
//
// CounterWorld creates N entities. Each tick, every entity adds a deterministic
// random value to its accumulator. The model contains no domain meaning; it
// exists only to provide a predictable, measurable workload. It is expressed as
// a SystemModel so the engine can parallelize the per-tick increment across
// entities while preserving determinism.
package counter

import (
	"context"
	"encoding/binary"
	"encoding/json"
	"hash/fnv"

	"github.com/ripper19/simulator/pkg/model"
	"github.com/ripper19/simulator/pkg/rng"
	"github.com/ripper19/simulator/pkg/simulation"
)

// ModelID is the stable identifier for CounterWorld.
const ModelID = "counter"

// Value is the accumulator component.
type Value struct{ V uint64 }

// RandState is the per-entity random stream state component.
type RandState struct{ State uint64 }

// CounterWorld is the model. It keeps its component IDs and columns cached
// after Initialize so systems perform no registry lookups during a tick.
type CounterWorld struct {
	// N is the number of entities to create.
	N int
	// IncrementMax bounds each tick's random increment to [0, IncrementMax).
	// It defaults to 1000 when zero.
	IncrementMax uint64

	valueID  simulation.ComponentID
	randID   simulation.ComponentID
	valueCol *simulation.Column[Value]
	randCol  *simulation.Column[RandState]
}

// Metadata describes CounterWorld.
func (m *CounterWorld) Metadata() model.Metadata {
	return model.Metadata{
		ID:          ModelID,
		Name:        "CounterWorld",
		Version:     "1.0.0",
		Description: "Deterministic tick model: N counters each accumulate a random increment per tick.",
		Mode:        model.ModeTick,
	}
}

// Initialize creates N entities, each with a zero value and a per-entity
// random stream derived from the simulation seed.
func (m *CounterWorld) Initialize(ctx context.Context, w *simulation.World) error {
	m.valueID, m.valueCol = simulation.RegisterComponent[Value](w.Components, "counter.value")
	m.randID, m.randCol = simulation.RegisterComponent[RandState](w.Components, "counter.rand")

	seed := w.Random.Seed()
	for i := 0; i < m.N; i++ {
		e := w.Entities.Create()
		stream := rng.DeriveU64(seed, uint64(i)+1)
		m.valueCol.Set(e, Value{})
		m.randCol.Set(e, RandState{State: stream.State()})
	}
	return nil
}

// Systems returns the single increment system.
func (m *CounterWorld) Systems() []simulation.System {
	return []simulation.System{&counterSystem{m: m}}
}

// counterConfig is the JSON-serializable model configuration.
type counterConfig struct {
	N            int    `json:"n"`
	IncrementMax uint64 `json:"increment_max"`
}

// SnapshotConfig implements simulation.SnapshotModel.
func (m *CounterWorld) SnapshotConfig() any {
	return counterConfig{N: m.N, IncrementMax: m.IncrementMax}
}

// RestoreConfig implements simulation.SnapshotModel.
func (m *CounterWorld) RestoreConfig(raw json.RawMessage) error {
	return m.applyConfig(raw)
}

// Configure implements simulation.ConfigurableModel.
func (m *CounterWorld) Configure(raw json.RawMessage) error {
	return m.applyConfig(raw)
}

func (m *CounterWorld) applyConfig(raw json.RawMessage) error {
	if len(raw) == 0 {
		return nil
	}
	var c counterConfig
	if err := json.Unmarshal(raw, &c); err != nil {
		return err
	}
	m.N = c.N
	m.IncrementMax = c.IncrementMax
	return nil
}

// counterSystem advances every entity's random stream and adds the resulting
// value to its accumulator. It is fully per-entity independent, so the
// scheduler may shard it across workers without changing the result.
type counterSystem struct {
	m *CounterWorld
}

func (s *counterSystem) Name() string { return "counter.increment" }

func (s *counterSystem) Reads() []simulation.ComponentID {
	return []simulation.ComponentID{s.m.valueID, s.m.randID}
}

func (s *counterSystem) Writes() []simulation.ComponentID {
	return []simulation.ComponentID{s.m.valueID, s.m.randID}
}

func (s *counterSystem) Run(ctx context.Context, w *simulation.World, shard []simulation.EntityID) error {
	max := s.m.IncrementMax
	if max == 0 {
		max = 1000
	}
	for _, e := range shard {
		rs, ok := s.m.randCol.GetShard(e)
		if !ok {
			continue
		}
		r := rng.New(rs.State)
		inc := r.Uint64n(max)
		v, _ := s.m.valueCol.GetShard(e)
		v.V += inc
		s.m.valueCol.SetShard(e, v)
		s.m.randCol.SetShard(e, RandState{State: r.State()})
	}
	return nil
}

// Fingerprint returns a 64-bit hash of the world state (entity IDs and their
// accumulated values) in deterministic order. Two runs with the same seed and
// configuration must produce the same fingerprint regardless of worker count.
func (m *CounterWorld) Fingerprint(w *simulation.World) uint64 {
	h := fnv.New64a()
	var b [16]byte
	w.Entities.Each(func(e simulation.EntityID) {
		v, ok := m.valueCol.Get(e)
		if !ok {
			return
		}
		binary.LittleEndian.PutUint64(b[0:8], uint64(e))
		binary.LittleEndian.PutUint64(b[8:16], v.V)
		h.Write(b[:])
	})
	return h.Sum64()
}
