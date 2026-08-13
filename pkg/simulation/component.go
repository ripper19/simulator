package simulation

import (
	"fmt"
	"sync"
)

// ComponentID identifies a registered component type. IDs are assigned
// deterministically in registration order, so a model that registers its
// components in a fixed order sees the same IDs across runs.
type ComponentID uint32

// ComponentStore holds typed component columns keyed by ComponentID. Each
// component type is stored in its own SoA Column[T], so the store itself is
// type-agnostic: it only knows that columns exist, while type-safe access is
// provided through the generic free functions below.
type ComponentStore struct {
	mu     sync.RWMutex
	names  []string
	byName map[string]ComponentID
	cols   map[ComponentID]any
}

// NewComponentStore returns an empty component store.
func NewComponentStore() *ComponentStore {
	return &ComponentStore{
		byName: make(map[string]ComponentID),
		cols:   make(map[ComponentID]any),
	}
}

// Register assigns a ComponentID to a name, creating it on first use and
// returning the same ID on subsequent calls. Registration order is therefore
// deterministic for a model with a fixed registration sequence.
func (s *ComponentStore) Register(name string) ComponentID {
	s.mu.Lock()
	defer s.mu.Unlock()
	if id, ok := s.byName[name]; ok {
		return id
	}
	id := ComponentID(len(s.names))
	s.names = append(s.names, name)
	s.byName[name] = id
	return id
}

// Name returns the registered name for a component ID, or "" if unknown.
func (s *ComponentStore) Name(id ComponentID) string {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if int(id) >= len(s.names) {
		return ""
	}
	return s.names[id]
}

// columnOf returns the typed column for id, creating it on demand. It panics
// if the component was previously bound to a different type T, which is a
// programming error caught early.
func columnOf[T any](store *ComponentStore, id ComponentID, create bool) *Column[T] {
	store.mu.RLock()
	col, ok := store.cols[id]
	store.mu.RUnlock()
	if ok {
		return col.(*Column[T])
	}
	if !create {
		return nil
	}
	store.mu.Lock()
	defer store.mu.Unlock()
	if col, ok := store.cols[id]; ok {
		return col.(*Column[T])
	}
	c := &Column[T]{}
	store.cols[id] = c
	return c
}

// ColumnOf returns the typed column for id, creating it if it does not yet
// exist. Panics on a type mismatch (e.g. requesting ColumnOf[Value] for an ID
// previously bound to a different type).
func ColumnOf[T any](store *ComponentStore, id ComponentID) *Column[T] {
	return columnOf[T](store, id, true)
}

// RegisterComponent registers a component by name and returns its ID together
// with its typed column, binding the name to type T on first use. This is the
// idiomatic way for a model to declare a component type.
func RegisterComponent[T any](store *ComponentStore, name string) (ComponentID, *Column[T]) {
	id := store.Register(name)
	return id, columnOf[T](store, id, true)
}

// Column is a struct-of-arrays column holding the values of one component type
// across entities. It uses a sparse-set index: data and dense are parallel
// arrays indexed by "dense position", and sparse maps an entity index to its
// dense position (+1, with 0 meaning absent).
//
// This layout gives O(1) get/set/remove, cache-friendly iteration over data,
// and no boxing of values. The cost is one int32 sparse entry per ever-created
// entity index per component type.
type Column[T any] struct {
	mu     sync.RWMutex
	sparse []int32
	dense  []EntityID
	data   []T
}

// Set writes v for entity e, adding it if absent.
func (c *Column[T]) Set(e EntityID, v T) {
	c.mu.Lock()
	defer c.mu.Unlock()
	idx := e.Index()
	if int(idx) >= len(c.sparse) {
		c.sparse = growInt32(c.sparse, idx)
	}
	if c.sparse[idx] == 0 {
		c.sparse[idx] = int32(len(c.data) + 1)
		c.dense = append(c.dense, e)
		c.data = append(c.data, v)
		return
	}
	c.data[c.sparse[idx]-1] = v
}

// Get returns the value for entity e and whether it is present.
func (c *Column[T]) Get(e EntityID) (T, bool) {
	c.mu.RLock()
	defer c.mu.RUnlock()
	idx := e.Index()
	if int(idx) >= len(c.sparse) || c.sparse[idx] == 0 {
		var zero T
		return zero, false
	}
	return c.data[c.sparse[idx]-1], true
}

// MustGet returns the value for entity e, panicking if it is absent.
func (c *Column[T]) MustGet(e EntityID) T {
	v, ok := c.Get(e)
	if !ok {
		panic(fmt.Sprintf("simulation: component %T not present on %s", v, e))
	}
	return v
}

// Has reports whether entity e has this component.
func (c *Column[T]) Has(e EntityID) bool {
	c.mu.RLock()
	defer c.mu.RUnlock()
	idx := e.Index()
	return int(idx) < len(c.sparse) && c.sparse[idx] != 0
}

// Remove deletes the component for entity e and returns whether it was present.
func (c *Column[T]) Remove(e EntityID) bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	idx := e.Index()
	if int(idx) >= len(c.sparse) || c.sparse[idx] == 0 {
		return false
	}
	pos := int(c.sparse[idx]) - 1
	last := len(c.data) - 1
	if pos != last {
		moved := c.dense[last]
		c.data[pos] = c.data[last]
		c.dense[pos] = moved
		c.sparse[moved.Index()] = int32(pos + 1)
	}
	c.data = c.data[:last]
	c.dense = c.dense[:last]
	c.sparse[idx] = 0
	return true
}

// Each calls fn for every entity that has this component, in dense (insertion)
// order. The callback must not modify the column's membership (no Set of a new
// entity, no Remove); reading and writing the value of existing members is safe
// only if the caller holds no lock, which is the case here.
func (c *Column[T]) Each(fn func(EntityID, T)) {
	c.mu.RLock()
	dense := c.dense
	data := c.data
	c.mu.RUnlock()
	for i := range dense {
		fn(dense[i], data[i])
	}
}

// Len returns the number of entities that have this component.
func (c *Column[T]) Len() int {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return len(c.data)
}

func growInt32(s []int32, idx uint32) []int32 {
	n := len(s)
	target := int(idx) + 1
	if n >= target {
		return s
	}
	capNeed := target
	if capNeed < n*2 {
		capNeed = n * 2
	}
	ns := make([]int32, target, capNeed)
	copy(ns, s)
	return ns
}
