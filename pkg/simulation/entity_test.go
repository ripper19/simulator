package simulation

import "testing"

func TestEntityCreateAliveDestroy(t *testing.T) {
	m := NewEntityManager()
	if m.Len() != 0 {
		t.Fatalf("expected 0 entities, got %d", m.Len())
	}
	a := m.Create()
	b := m.Create()
	if !m.Alive(a) || !m.Alive(b) {
		t.Fatal("created entities must be alive")
	}
	if a == b {
		t.Fatal("distinct entities must have distinct IDs")
	}
	if m.Len() != 2 {
		t.Fatalf("expected 2 entities, got %d", m.Len())
	}
	if !m.Destroy(a) {
		t.Fatal("Destroy should succeed for live entity")
	}
	if m.Alive(a) {
		t.Fatal("destroyed entity must not be alive")
	}
	if !m.Alive(b) {
		t.Fatal("sibling entity must remain alive")
	}
	if m.Len() != 1 {
		t.Fatalf("expected 1 entity, got %d", m.Len())
	}
	if m.Destroy(a) {
		t.Fatal("double Destroy should return false")
	}
}

func TestEntityGenerationReuse(t *testing.T) {
	m := NewEntityManager()
	a := m.Create()
	m.Destroy(a)
	c := m.Create()
	if c.Index() != a.Index() {
		t.Fatalf("expected index reuse: %d vs %d", c.Index(), a.Index())
	}
	if c.Version() <= a.Version() {
		t.Fatalf("reused index must increment version: %d -> %d", a.Version(), c.Version())
	}
	if m.Alive(a) {
		t.Fatal("stale ID must not resolve as alive")
	}
	if !m.Alive(c) {
		t.Fatal("new ID must be alive")
	}
}

func TestEntityIDsDeterministicOrder(t *testing.T) {
	m := NewEntityManager()
	ids := make([]EntityID, 0, 10)
	for i := 0; i < 10; i++ {
		ids = append(ids, m.Create())
	}
	var got []EntityID
	m.Each(func(e EntityID) { got = append(got, e) })
	if len(got) != len(ids) {
		t.Fatalf("iteration length %d != %d", len(got), len(ids))
	}
	for i := range ids {
		if got[i] != ids[i] {
			t.Fatalf("iteration order mismatch at %d", i)
		}
	}
}
