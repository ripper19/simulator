// Package logistics is an example tick model: orders are dispatched from
// warehouses and delivered by vehicles with limited capacity.
package logistics

import (
	"context"

	"github.com/ripper19/simulator/pkg/model"
	"github.com/ripper19/simulator/pkg/rng"
	"github.com/ripper19/simulator/pkg/simulation"
)

// Inventory is a warehouse's stock of units.
type Inventory struct{ Units int }

// Capacity is a vehicle's load limit.
type Capacity struct{ Max, Load int }

// Order is a pending delivery, in remaining units.
type Order struct{ Remaining int }

// Logistics dispatches orders to vehicles each tick.
type Logistics struct {
	Warehouses int
	Vehicles   int
	Orders     int

	invID  simulation.ComponentID
	invCol *simulation.Column[Inventory]
	capID  simulation.ComponentID
	capCol *simulation.Column[Capacity]
	ordID  simulation.ComponentID
	ordCol *simulation.Column[Order]

	rng         rng.RNG
	assignments map[simulation.EntityID]simulation.EntityID // vehicle -> order
	delivered   int
}

// Metadata describes the logistics model.
func (m *Logistics) Metadata() model.Metadata {
	return model.Metadata{
		ID:          "logistics",
		Name:        "Logistics",
		Version:     "1.0.0",
		Description: "Orders are dispatched and delivered by vehicles.",
		Mode:        model.ModeTick,
	}
}

// Initialize creates warehouses, vehicles, and orders.
func (m *Logistics) Initialize(ctx context.Context, w *simulation.World) error {
	m.invID, m.invCol = simulation.RegisterComponent[Inventory](w.Components, "log.inventory")
	m.capID, m.capCol = simulation.RegisterComponent[Capacity](w.Components, "log.capacity")
	m.ordID, m.ordCol = simulation.RegisterComponent[Order](w.Components, "log.order")
	m.rng = w.Random.StreamU64(1)
	m.assignments = make(map[simulation.EntityID]simulation.EntityID)

	for i := 0; i < m.Warehouses; i++ {
		m.invCol.Set(w.Entities.Create(), Inventory{Units: 1000})
	}
	for i := 0; i < m.Vehicles; i++ {
		m.capCol.Set(w.Entities.Create(), Capacity{Max: 3})
	}
	for i := 0; i < m.Orders; i++ {
		m.ordCol.Set(w.Entities.Create(), Order{Remaining: 1 + int(m.rng.Uint64n(5))})
	}
	return nil
}

// Step dispatches and delivers one tick of work.
func (m *Logistics) Step(ctx context.Context, w *simulation.World) error {
	m.assign(w)
	m.deliver(w)
	return nil
}

// assign dispatches unassigned orders to vehicles with free capacity.
func (m *Logistics) assign(w *simulation.World) {
	for _, o := range w.Components.Entities(m.ordID) {
		if _, busy := m.assignmentsReverse(o); busy {
			continue
		}
		if v, ok := m.freeVehicle(w); ok {
			m.assignments[v] = o
			c, _ := m.capCol.Get(v)
			c.Load++
			m.capCol.Set(v, c)
		}
	}
}

// deliver advances each assigned order and frees completed ones.
func (m *Logistics) deliver(w *simulation.World) {
	for v, o := range m.assignments {
		ord, ok := m.ordCol.Get(o)
		if !ok {
			delete(m.assignments, v)
			continue
		}
		ord.Remaining--
		if ord.Remaining <= 0 {
			m.delivered++
			w.Entities.Destroy(o)
			m.ordCol.Remove(o)
			c, _ := m.capCol.Get(v)
			c.Load--
			m.capCol.Set(v, c)
			delete(m.assignments, v)
		} else {
			m.ordCol.Set(o, ord)
		}
	}
}

func (m *Logistics) freeVehicle(w *simulation.World) (simulation.EntityID, bool) {
	for _, v := range w.Components.Entities(m.capID) {
		if _, busy := m.assignments[v]; busy {
			continue
		}
		if c, ok := m.capCol.Get(v); ok && c.Load < c.Max {
			return v, true
		}
	}
	return 0, false
}

func (m *Logistics) assignmentsReverse(o simulation.EntityID) (simulation.EntityID, bool) {
	for v, oo := range m.assignments {
		if oo == o {
			return v, true
		}
	}
	return 0, false
}

// Delivered returns the number of completed orders.
func (m *Logistics) Delivered() int { return m.delivered }
