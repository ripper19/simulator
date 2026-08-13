package simulation

import "sync"

// Clock tracks simulation time. It carries both a tick counter (for
// discrete-tick models) and a floating-point time (for discrete-event models),
// so a single clock can serve either mode. Only the runtime (or a model, via
// scheduled events) advances the clock; this keeps advancement deterministic.
type Clock struct {
	mu   sync.RWMutex
	tick uint64
	time float64
}

// NewClock returns a clock at tick 0 and time 0.
func NewClock() *Clock { return &Clock{} }

// Tick returns the current tick index.
func (c *Clock) Tick() uint64 {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.tick
}

// Time returns the current simulation time.
func (c *Clock) Time() float64 {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.time
}

// Advance increments the tick counter by one.
func (c *Clock) Advance() {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.tick++
}

// AdvanceTime sets the simulation time (discrete-event advancement).
func (c *Clock) AdvanceTime(t float64) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.time = t
}
