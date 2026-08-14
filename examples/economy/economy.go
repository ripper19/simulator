// Package economy is an example model where a price series emerges from
// aggregate supply and demand. A configurable intervention (supply shock or
// price cap) visibly changes the price trajectory, all deterministically. It is
// an illustrative toy: prices are not stabilized, so sustained shocks produce
// unbounded price movement.
package economy

import (
	"context"
	"encoding/json"
	"math"

	"github.com/ripper19/simulator/pkg/model"
	"github.com/ripper19/simulator/pkg/rng"
	"github.com/ripper19/simulator/pkg/simulation"
)

// Config is the JSON-configurable economy scenario.
type Config struct {
	Agents          int     `json:"agents"`
	Productivity    float64 `json:"productivity"` // goods produced per agent per tick
	Demand          float64 `json:"demand"`       // goods consumed per agent per tick
	PriceAdjust     float64 `json:"price_adjust"` // price sensitivity to excess demand
	PriceCap        float64 `json:"price_cap"`    // 0 = no cap
	SupplyShockTick int     `json:"supply_shock_tick"`
	ShockFactor     float64 `json:"shock_factor"` // production multiplier after the shock
}

func (c Config) withDefaults() Config {
	if c.Agents <= 0 {
		c.Agents = 100
	}
	if c.Productivity <= 0 {
		c.Productivity = 10
	}
	if c.Demand <= 0 {
		c.Demand = 10
	}
	if c.PriceAdjust <= 0 {
		c.PriceAdjust = 0.05
	}
	if c.ShockFactor <= 0 {
		c.ShockFactor = 0.5
	}
	return c
}

// Agent holds cash and goods.
type Agent struct {
	Cash  float64
	Goods float64
}

// Economy runs the market.
type Economy struct {
	cfg Config

	agentID  simulation.ComponentID
	agentCol *simulation.Column[Agent]

	price       float64
	totalWealth float64
	prices      []float64
	rng         rng.RNG
}

// Metadata describes the economy model.
func (m *Economy) Metadata() model.Metadata {
	return model.Metadata{
		ID:          "economy",
		Name:        "Economy",
		Version:     "1.0.0",
		Description: "A price series emerges from aggregate supply and demand.",
		Mode:        model.ModeTick,
	}
}

// Configure applies the scenario configuration.
func (m *Economy) Configure(raw json.RawMessage) error {
	var c Config
	if len(raw) == 0 {
		c = Config{}
	} else if err := json.Unmarshal(raw, &c); err != nil {
		return err
	}
	m.cfg = c.withDefaults()
	return nil
}

// Initialize creates agents and sets the initial price.
func (m *Economy) Initialize(ctx context.Context, w *simulation.World) error {
	if m.cfg.Agents == 0 {
		m.cfg = m.cfg.withDefaults()
	}
	m.agentID, m.agentCol = simulation.RegisterComponent[Agent](w.Components, "econ.agent")
	m.rng = w.Random.StreamU64(1)
	m.price = 1.0
	for i := 0; i < m.cfg.Agents; i++ {
		m.agentCol.Set(w.Entities.Create(), Agent{Cash: 100, Goods: m.cfg.Productivity})
	}
	return nil
}

// Step clears the market once.
func (m *Economy) Step(ctx context.Context, w *simulation.World) error {
	tick := int(w.Clock.Tick())
	factor := 1.0
	if m.cfg.SupplyShockTick > 0 && tick >= m.cfg.SupplyShockTick {
		factor = m.cfg.ShockFactor
	}

	// Production and demand.
	prods := make([]float64, 0, m.cfg.Agents)
	supply := 0.0
	m.agentCol.Each(func(id simulation.EntityID, a Agent) {
		p := m.cfg.Productivity * (0.9 + 0.2*m.rng.Float64()) * factor
		prods = append(prods, p)
		supply += p
	})
	demand := float64(m.cfg.Agents) * m.cfg.Demand

	// Price moves with excess demand; a cap bounds it.
	if supply > 0 {
		m.price *= 1 + m.cfg.PriceAdjust*(demand-supply)/supply
	}
	if m.cfg.PriceCap > 0 && m.price > m.cfg.PriceCap {
		m.price = m.cfg.PriceCap
	}
	m.price = math.Max(0.01, m.price)

	// Agents trade the difference between production and demand at market price.
	m.totalWealth = 0
	i := 0
	m.agentCol.Each(func(id simulation.EntityID, a Agent) {
		net := prods[i] - m.cfg.Demand
		a.Cash += net * m.price
		i++
		m.totalWealth += a.Cash
		m.agentCol.Set(id, a)
	})
	m.prices = append(m.prices, m.price)
	return nil
}

// Metrics reports the measured price outcomes.
func (m *Economy) Metrics() map[string]float64 {
	if len(m.prices) == 0 {
		return map[string]float64{}
	}
	sum, min, max := 0.0, m.prices[0], m.prices[0]
	for _, p := range m.prices {
		sum += p
		min = math.Min(min, p)
		max = math.Max(max, p)
	}
	return map[string]float64{
		"avg_price":    sum / float64(len(m.prices)),
		"final_price":  m.price,
		"min_price":    min,
		"max_price":    max,
		"total_wealth": m.totalWealth,
	}
}
