package simulation

import "github.com/ripper19/simulator/pkg/rng"

// RandomStreams provides deterministic random streams derived from the
// simulation seed. Streams are named and order-independent, so different parts
// of a model (and later, different execution shards) can draw randomness
// without depending on each other's call order.
type RandomStreams struct {
	seed uint64
}

// NewRandomStreams returns a stream source rooted at seed.
func NewRandomStreams(seed uint64) *RandomStreams {
	return &RandomStreams{seed: seed}
}

// Seed returns the master seed.
func (s *RandomStreams) Seed() uint64 { return s.seed }

// Stream returns a stream derived from the seed and the given key parts. The
// same key always yields the same stream; different keys yield independent
// streams.
func (s *RandomStreams) Stream(keys ...string) *rng.RNG {
	return rng.Derive(s.seed, keys...)
}

// StreamU64 returns a stream derived from the seed and a raw uint64 key, for
// hot paths such as per-entity streams.
func (s *RandomStreams) StreamU64(stream uint64) *rng.RNG {
	return rng.DeriveU64(s.seed, stream)
}
