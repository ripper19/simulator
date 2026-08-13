package simulation

import "testing"

type vec struct{ X, Y float64 }

func TestColumnSetGetHasRemove(t *testing.T) {
	store := NewComponentStore()
	id, col := RegisterComponent[vec](store, "vec")

	m := NewEntityManager()
	a := m.Create()
	b := m.Create()

	if col.Has(a) {
		t.Fatal("component should not be present before Set")
	}
	col.Set(a, vec{1, 2})
	if !col.Has(a) {
		t.Fatal("component should be present after Set")
	}
	v, ok := col.Get(a)
	if !ok || v != (vec{1, 2}) {
		t.Fatalf("Get mismatch: %v %v", v, ok)
	}
	if col.Len() != 1 {
		t.Fatalf("expected 1 component, got %d", col.Len())
	}

	// overwrite in place (should not grow)
	col.Set(a, vec{3, 4})
	v, _ = col.Get(a)
	if v != (vec{3, 4}) {
		t.Fatalf("overwrite mismatch: %v", v)
	}
	if col.Len() != 1 {
		t.Fatalf("overwrite should not change length: %d", col.Len())
	}

	col.Set(b, vec{5, 6})
	if col.Len() != 2 {
		t.Fatalf("expected 2 components, got %d", col.Len())
	}

	if !col.Remove(a) {
		t.Fatal("Remove should succeed")
	}
	if col.Has(a) {
		t.Fatal("component should be absent after Remove")
	}
	if !col.Has(b) {
		t.Fatal("sibling component should remain")
	}
	if col.Remove(a) {
		t.Fatal("double Remove should return false")
	}
	if id != store.Register("vec") {
		t.Fatal("Register should return the same ID on repeat")
	}
}

func TestComponentRegisterDeterministic(t *testing.T) {
	a := NewComponentStore()
	b := NewComponentStore()
	for _, name := range []string{"x", "y", "z"} {
		if a.Register(name) != b.Register(name) {
			t.Fatalf("component ID mismatch for %q", name)
		}
	}
}

func TestColumnEach(t *testing.T) {
	_, col := RegisterComponent[int](NewComponentStore(), "int")
	m := NewEntityManager()
	for i := 0; i < 5; i++ {
		e := m.Create()
		col.Set(e, i*10)
	}
	sum := 0
	col.Each(func(_ EntityID, v int) { sum += v })
	if sum != 100 {
		t.Fatalf("Each sum = %d, want 100", sum)
	}
}
