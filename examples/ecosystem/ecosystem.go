// Package ecosystem is an example tick model with entity creation and
// destruction (plants grow, animals eat, reproduce, and die). It uses the
// serial TickModel path, since entity membership changes are not allowed inside
// parallel system shards.
package ecosystem

import (
	"context"

	"github.com/ripper19/simulator/pkg/model"
	"github.com/ripper19/simulator/pkg/simulation"
)

// Energy is the shared energy component for animals and plants.
type Energy struct{ E int }

// Ecosystem is a simple ecosystem: plants grow, animals graze, reproduce when
// well-fed (subject to a carrying capacity), and die at zero energy.
type Ecosystem struct {
	Plants     int
	Animals    int
	MaxAnimals int // carrying capacity for density-dependent reproduction

	energyID  simulation.ComponentID
	energyCol *simulation.Column[Energy]
	animalTag simulation.Tag
	plantTag  simulation.Tag

	plantCursor int
}

// Metadata describes the ecosystem model.
func (m *Ecosystem) Metadata() model.Metadata {
	return model.Metadata{
		ID:          "ecosystem",
		Name:        "Ecosystem",
		Version:     "1.0.0",
		Description: "Plants grow; animals graze, reproduce, and die.",
		Mode:        model.ModeTick,
	}
}

// Initialize seeds plants and animals.
func (m *Ecosystem) Initialize(ctx context.Context, w *simulation.World) error {
	m.energyID, m.energyCol = simulation.RegisterComponent[Energy](w.Components, "eco.energy")
	m.animalTag = w.Tags.Register("animal")
	m.plantTag = w.Tags.Register("plant")

	stream := w.Random.StreamU64(1)
	for i := 0; i < m.Plants; i++ {
		e := w.Entities.Create()
		w.TagStore.Add(e, m.plantTag)
		m.energyCol.Set(e, Energy{E: 3 + int(stream.Uint64n(3))})
	}
	for i := 0; i < m.Animals; i++ {
		e := w.Entities.Create()
		w.TagStore.Add(e, m.animalTag)
		m.energyCol.Set(e, Energy{E: 4 + int(stream.Uint64n(4))})
	}
	return nil
}

// Step advances the ecosystem by one tick.
func (m *Ecosystem) Step(ctx context.Context, w *simulation.World) error {
	// Grow plants (up to a cap).
	plants := m.plantIDs(w)
	for _, p := range plants {
		v, _ := m.energyCol.Get(p)
		if v.E < 8 {
			v.E++
			m.energyCol.Set(p, v)
		}
	}

	// Animals graze on the next available plant (round-robin) and metabolize.
	animals := m.animalIDs(w)
	for _, a := range animals {
		v, _ := m.energyCol.Get(a)
		v.E--
		if len(plants) > 0 {
			p := plants[m.plantCursor%len(plants)]
			m.plantCursor++
			if pv, ok := m.energyCol.Get(p); ok && pv.E > 0 {
				pv.E--
				m.energyCol.Set(p, pv)
				v.E += 3
			}
		}
		m.energyCol.Set(a, v)
	}

	// Reproduce well-fed animals (up to carrying capacity) and collect the dead.
	canReproduce := len(animals) < m.MaxAnimals
	var dead []simulation.EntityID
	for _, a := range animals {
		v, _ := m.energyCol.Get(a)
		if v.E <= 0 {
			dead = append(dead, a)
		} else if canReproduce && v.E >= 10 {
			v.E -= 5
			m.energyCol.Set(a, v)
			child := w.Entities.Create()
			w.TagStore.Add(child, m.animalTag)
			m.energyCol.Set(child, Energy{E: 5})
		}
	}
	for _, p := range plants {
		if v, ok := m.energyCol.Get(p); ok && v.E <= 0 {
			dead = append(dead, p)
		}
	}
	for _, e := range dead {
		w.Entities.Destroy(e)
		w.TagStore.RemoveEntity(e)
		m.energyCol.Remove(e)
	}
	return nil
}

func (m *Ecosystem) plantIDs(w *simulation.World) []simulation.EntityID {
	var out []simulation.EntityID
	w.Entities.Each(func(e simulation.EntityID) {
		if w.TagStore.Has(e, m.plantTag) {
			out = append(out, e)
		}
	})
	return out
}

func (m *Ecosystem) animalIDs(w *simulation.World) []simulation.EntityID {
	var out []simulation.EntityID
	w.Entities.Each(func(e simulation.EntityID) {
		if w.TagStore.Has(e, m.animalTag) {
			out = append(out, e)
		}
	})
	return out
}
