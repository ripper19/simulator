// Package rng provides deterministic, seedable pseudo-random number generation
// for simulations. It is designed so that a given seed always produces the same
// sequence, and so that independent random streams can be derived from a single
// master seed without affecting each other.
//
// The core generator is splitmix64, a fast, high-quality 64-bit PRNG with no
// global state. Streams are derived by mixing the master seed with a stream key
// (either a sequence of string parts or a raw uint64), which makes stream
// assignment order-independent and therefore safe to use in parallel execution.
package rng

import (
	"encoding/binary"
	"hash/fnv"
	"math/bits"
)

const (
	golden = 0x9E3779B97F4A7C15
)

// RNG is a single deterministic random stream.
type RNG struct {
	state uint64
}

// New returns an RNG seeded with the given value. Seeding with 0 is valid;
// splitmix64 will still produce a well-distributed sequence. RNG is a value
// type: it is a single uint64, so creating and copying it allocates nothing.
func New(seed uint64) RNG {
	return RNG{state: seed}
}

// State returns the current internal state, allowing a stream to be snapshotted
// and restored later.
func (r RNG) State() uint64 { return r.state }

// Uint64 returns the next 64-bit random value and advances the stream.
func (r *RNG) Uint64() uint64 {
	r.state += golden
	z := r.state
	z = (z ^ (z >> 30)) * 0xBF58476D1CE4E5B9
	z = (z ^ (z >> 27)) * 0x94D049BB133111EB
	return z ^ (z >> 31)
}

// Uint64n returns a uniformly distributed random value in [0, n).
// It uses Lemire's unbiased bounded-range method so the result is unbiased.
// It panics if n is 0.
func (r *RNG) Uint64n(n uint64) uint64 {
	if n == 0 {
		panic("rng: Uint64n called with zero bound")
	}
	x := r.Uint64()
	hi, lo := bits.Mul64(x, n)
	if lo < n {
		threshold := -n % n
		for lo < threshold {
			x = r.Uint64()
			hi, lo = bits.Mul64(x, n)
		}
	}
	return hi
}

// Int63n returns a uniformly distributed random value in [0, n).
// It panics if n <= 0.
func (r *RNG) Int63n(n int64) int64 {
	if n <= 0 {
		panic("rng: Int63n called with non-positive bound")
	}
	return int64(r.Uint64n(uint64(n)))
}

// Float64 returns a uniformly distributed random float64 in [0, 1).
func (r *RNG) Float64() float64 {
	return float64(r.Uint64()>>11) * (1.0 / (1 << 53))
}

// Split returns a new child stream derived from the current stream state.
// Because it consumes state, Split must be called in a deterministic order.
// Prefer Derive or DeriveU64 when an order-independent named stream is needed.
func (r *RNG) Split() RNG {
	return New(r.Uint64())
}

// Derive returns an independent stream deterministically derived from a master
// seed and a sequence of key parts. The same (seed, keys...) always yields the
// same stream, and different keys yield unrelated streams. This is
// order-independent, so it is safe to derive per-entity or per-system streams.
func Derive(seed uint64, keys ...string) RNG {
	h := fnv.New64a()
	var b [8]byte
	binary.LittleEndian.PutUint64(b[:], seed)
	h.Write(b[:])
	for _, k := range keys {
		h.Write([]byte{0xff})
		h.Write([]byte(k))
	}
	return New(h.Sum64())
}

// DeriveU64 returns an independent stream derived from a master seed and a
// raw uint64 stream identifier. It avoids string hashing overhead, making it
// suitable for hot paths such as per-entity streams.
func DeriveU64(seed, stream uint64) RNG {
	return New(mix64(seed) ^ mix64(stream+golden))
}

// mix64 is the splitmix64 finalizer, a bijective mixing function. Applying it
// to distinct inputs yields distinct outputs, which keeps derived streams
// distinct.
func mix64(z uint64) uint64 {
	z = (z ^ (z >> 30)) * 0xBF58476D1CE4E5B9
	z = (z ^ (z >> 27)) * 0x94D049BB133111EB
	return z ^ (z >> 31)
}
