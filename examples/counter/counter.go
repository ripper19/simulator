// Package counter implements CounterWorld, a deliberately trivial test model
// used to exercise the engine's determinism, parallelism, snapshots, replay,
// and performance characteristics.
//
// CounterWorld creates N entities. Each tick, every entity adds a deterministic
// random value to its accumulator. The model contains no domain meaning; it
// exists only to provide a predictable, measurable workload.
package counter

import (
	"context"
	"encoding/binary"
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

// CounterWorld is the model. It keeps its component IDs cached after
// Initialize so Step performs no registry lookups.
type CounterWorld struct {
	// N is the number of entities to create.
	N int
	// IncrementMax bounds each tick's random increment to [0, IncrementMax).
	// It defaults to 1000 when zero.
	IncrementMax uint64

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
	_, m.valueCol = simulation.RegisterComponent[Value](w.Components, "counter.value")
	_, m.randCol = simulation.RegisterComponent[RandState](w.Components, "counter.rand")

	seed := w.Random.Seed()
	for i := 0; i < m.N; i++ {
		e := w.Entities.Create()
		stream := rng.DeriveU64(seed, uint64(i)+1)
		m.valueCol.Set(e, Value{})
		m.randCol.Set(e, RandState{State: stream.State()})
	}
	return nil
}

// Step advances every entity's random stream and adds the resulting value to
// its accumulator.
func (m *CounterWorld) Step(ctx context.Context, w *simulation.World) error {
	max := m.IncrementMax
	if max == 0 {
		max = 1000
	}
	w.Entities.Each(func(e simulation.EntityID) {
		rs := m.randCol.MustGet(e)
		r := rng.New(rs.State)
		inc := r.Uint64n(max)
		v := m.valueCol.MustGet(e)
		v.V += inc
		m.valueCol.Set(e, v)
		m.randCol.Set(e, RandState{State: r.State()})
	})
	return nil
}

// Fingerprint returns a 64-bit hash of the world state (entity IDs and their
// accumulated values) in deterministic order. Two runs with the same seed and
// configuration must produce the same fingerprint.
func (m *CounterWorld) Fingerprint(w *simulation.World) uint64 {
	h := fnv.New64a()
	var b [16]byte
	w.Entities.Each(func(e simulation.EntityID) {
		v := m.valueCol.MustGet(e)
		binary.LittleEndian.PutUint64(b[0:8], uint64(e))
		binary.LittleEndian.PutUint64(b[8:16], v.V)
		h.Write(b[:])
	})
	return h.Sum64()
}
