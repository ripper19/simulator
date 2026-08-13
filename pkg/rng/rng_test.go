package rng

import "testing"

func TestDeterministicSequence(t *testing.T) {
	a := New(42)
	b := New(42)
	for i := 0; i < 1000; i++ {
		if a.Uint64() != b.Uint64() {
			t.Fatalf("sequences diverged at %d", i)
		}
	}
}

func TestDifferentSeedsDiffer(t *testing.T) {
	a := New(1)
	b := New(2)
	same := 0
	for i := 0; i < 100; i++ {
		if a.Uint64() == b.Uint64() {
			same++
		}
	}
	if same > 1 {
		t.Fatalf("distinct seeds produced %d identical values", same)
	}
}

func TestUint64nBounds(t *testing.T) {
	r := New(7)
	for i := 0; i < 100000; i++ {
		if v := r.Uint64n(10); v >= 10 {
			t.Fatalf("Uint64n(10) returned %d", v)
		}
	}
}

func TestInt63nBounds(t *testing.T) {
	r := New(8)
	for i := 0; i < 100000; i++ {
		if v := r.Int63n(7); v < 0 || v >= 7 {
			t.Fatalf("Int63n(7) returned %d", v)
		}
	}
}

func TestFloat64Range(t *testing.T) {
	r := New(9)
	for i := 0; i < 10000; i++ {
		if v := r.Float64(); v < 0 || v >= 1 {
			t.Fatalf("Float64 returned %f", v)
		}
	}
}

func TestDeriveStableAndDistinct(t *testing.T) {
	a := Derive(123, "entity", "0")
	b := Derive(123, "entity", "0")
	c := Derive(123, "entity", "1")
	for i := 0; i < 100; i++ {
		if a.Uint64() != b.Uint64() {
			t.Fatalf("Derive not stable at %d", i)
		}
	}
	if a.State() == c.State() {
		t.Fatalf("distinct keys produced identical streams")
	}
}

func TestDeriveU64Distinct(t *testing.T) {
	a := DeriveU64(42, 0)
	b := DeriveU64(42, 1)
	if a.State() == b.State() {
		t.Fatalf("distinct stream ids produced identical streams")
	}
}
