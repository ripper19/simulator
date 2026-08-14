package simulation

import "sync"

// Tag identifies a named tag. Tags are lightweight, engine-agnostic labels that
// models attach to entities for filtering (for example "movable", "blocked",
// or "server"). The engine does not assign meaning to any tag.
type Tag uint32

// TagRegistry maps tag names to stable Tag IDs, assigned deterministically in
// registration order.
type TagRegistry struct {
	mu     sync.RWMutex
	names  []string
	byName map[string]Tag
}

// NewTagRegistry returns an empty tag registry.
func NewTagRegistry() *TagRegistry {
	return &TagRegistry{byName: make(map[string]Tag)}
}

// Register returns the Tag for name, creating it on first use.
func (t *TagRegistry) Register(name string) Tag {
	t.mu.Lock()
	defer t.mu.Unlock()
	if id, ok := t.byName[name]; ok {
		return id
	}
	id := Tag(len(t.names))
	t.names = append(t.names, name)
	t.byName[name] = id
	return id
}

// Name returns the name for a Tag, or "" if unknown.
func (t *TagRegistry) Name(tag Tag) string {
	t.mu.RLock()
	defer t.mu.RUnlock()
	if int(tag) >= len(t.names) {
		return ""
	}
	return t.names[tag]
}

// snapshot returns the tag names in ID order.
func (t *TagRegistry) snapshot() []string {
	t.mu.RLock()
	defer t.mu.RUnlock()
	return append([]string(nil), t.names...)
}

// restore rebuilds the tag registry from the given names (in ID order).
func (t *TagRegistry) restore(names []string) {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.names = append([]string(nil), names...)
	t.byName = make(map[string]Tag, len(names))
	for i, n := range names {
		t.byName[n] = Tag(i)
	}
}

// TagStore stores the set of tags attached to each entity. Tags are optional
// and lazily allocated per entity; they use a growable bitset so that a single
// uint64 covers up to 64 tags with O(1) membership tests and small memory.
type TagStore struct {
	mu   sync.RWMutex
	sets map[EntityID][]uint64
}

// NewTagStore returns an empty tag store.
func NewTagStore() *TagStore {
	return &TagStore{sets: make(map[EntityID][]uint64)}
}

// Add attaches tag to entity e (idempotent).
func (s *TagStore) Add(e EntityID, tag Tag) {
	s.mu.Lock()
	defer s.mu.Unlock()
	word := int(tag) / 64
	bit := uint(tag) % 64
	words := s.sets[e]
	if len(words) <= word {
		nw := make([]uint64, word+1)
		copy(nw, words)
		words = nw
		s.sets[e] = words
	}
	words[word] |= 1 << bit
}

// Remove detaches tag from entity e (idempotent).
func (s *TagStore) Remove(e EntityID, tag Tag) {
	s.mu.Lock()
	defer s.mu.Unlock()
	words, ok := s.sets[e]
	if !ok {
		return
	}
	word := int(tag) / 64
	if word < len(words) {
		words[word] &^= 1 << (uint(tag) % 64)
	}
}

// Has reports whether entity e has tag.
func (s *TagStore) Has(e EntityID, tag Tag) bool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	words, ok := s.sets[e]
	if !ok {
		return false
	}
	word := int(tag) / 64
	return word < len(words) && words[word]&(1<<(uint(tag)%64)) != 0
}

// RemoveEntity drops all tags for entity e.
func (s *TagStore) RemoveEntity(e EntityID) {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.sets, e)
}

// snapshot returns the per-entity tag bitsets.
func (s *TagStore) snapshot() map[EntityID][]uint64 {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make(map[EntityID][]uint64, len(s.sets))
	for e, words := range s.sets {
		out[e] = append([]uint64(nil), words...)
	}
	return out
}

// restore rebuilds the tag store from the given per-entity bitsets.
func (s *TagStore) restore(sets map[EntityID][]uint64) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.sets = make(map[EntityID][]uint64, len(sets))
	for e, words := range sets {
		s.sets[e] = append([]uint64(nil), words...)
	}
}
