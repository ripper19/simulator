package simulation

import (
	"encoding/json"
	"strconv"
	"sync"
)

// EntityID is a stable identifier for an entity. The high 32 bits are the
// entity index and the low 32 bits are a generation counter that increments
// each time the index is reused, so a stale EntityID can never be mistaken for
// a live entity.
type EntityID uint64

// Index returns the entity's index (the high 32 bits).
func (e EntityID) Index() uint32 { return uint32(e >> 32) }

// Version returns the entity's generation (the low 32 bits).
func (e EntityID) Version() uint32 { return uint32(e) }

func (e EntityID) String() string {
	return "entity:" + strconv.FormatUint(uint64(e), 10)
}

// MarshalJSON serializes an EntityID as a decimal string, which is exact for
// the full 64-bit value (JSON numbers would lose precision above 2^53).
func (e EntityID) MarshalJSON() ([]byte, error) {
	return json.Marshal(strconv.FormatUint(uint64(e), 10))
}

// UnmarshalJSON deserializes an EntityID from a decimal string.
func (e *EntityID) UnmarshalJSON(b []byte) error {
	var s string
	if err := json.Unmarshal(b, &s); err != nil {
		return err
	}
	v, err := strconv.ParseUint(s, 10, 64)
	if err != nil {
		return err
	}
	*e = EntityID(v)
	return nil
}

// EntityManager allocates, tracks, and reclaims entities using a sparse-set
// layout: a dense slice holds live entities in creation order, and a sparse
// slice (indexed by entity index) maps an index to its position in the dense
// slice. Generation counters prevent stale references from resolving.
//
// Iteration order is deterministic for a given sequence of create/destroy
// operations (creation order). Callers that require strict cross-run ordering
// independent of operation history must iterate over a sorted snapshot.
type EntityManager struct {
	mu       sync.RWMutex
	versions []uint32   // per-index generation counter
	sparse   []int32    // index -> dense position + 1 (0 means absent)
	dense    []EntityID // dense position -> live entity (creation order)
	free     []uint32   // recycled indices
	next     uint32     // next index to allocate when free is empty
	count    int
	bump     func() // optional structural-revision callback
}

// NewEntityManager returns an empty entity manager.
func NewEntityManager() *EntityManager {
	return &EntityManager{
		versions: make([]uint32, 1), // reserve index 0
		sparse:   make([]int32, 1),
	}
}

// Create allocates a new entity and returns its unique ID.
func (m *EntityManager) Create() EntityID {
	m.mu.Lock()
	defer m.mu.Unlock()

	var index uint32
	if n := len(m.free); n > 0 {
		index = m.free[n-1]
		m.free = m.free[:n-1]
	} else {
		index = m.next
		m.next++
		for int(index) >= len(m.versions) {
			m.versions = append(m.versions, 0)
			m.sparse = append(m.sparse, 0)
		}
	}

	m.versions[index]++
	id := EntityID(uint64(index)<<32 | uint64(m.versions[index]))

	pos := len(m.dense)
	m.dense = append(m.dense, id)
	m.sparse[index] = int32(pos + 1)
	m.count++
	if m.bump != nil {
		m.bump()
	}
	return id
}

// Alive reports whether id refers to a live entity.
func (m *EntityManager) Alive(id EntityID) bool {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.aliveLocked(id)
}

func (m *EntityManager) aliveLocked(id EntityID) bool {
	idx := id.Index()
	if int(idx) >= len(m.versions) {
		return false
	}
	return m.versions[idx] == id.Version() && m.sparse[idx] != 0
}

// Destroy removes a live entity and returns true. It returns false if id does
// not refer to a live entity. The index is recycled for later reuse with a new
// generation.
func (m *EntityManager) Destroy(id EntityID) bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	if !m.aliveLocked(id) {
		return false
	}
	idx := id.Index()
	pos := int(m.sparse[idx]) - 1
	last := len(m.dense) - 1
	if pos != last {
		moved := m.dense[last]
		m.dense[pos] = moved
		m.sparse[moved.Index()] = int32(pos + 1)
	}
	m.dense = m.dense[:last]
	m.sparse[idx] = 0
	m.free = append(m.free, idx)
	m.count--
	if m.bump != nil {
		m.bump()
	}
	return true
}

// Each calls fn for every live entity, in deterministic creation order. The
// callback must not call Create or Destroy; the underlying slice is not copied,
// so structural changes during iteration are unsupported and undefined.
func (m *EntityManager) Each(fn func(EntityID)) {
	m.mu.RLock()
	dense := m.dense
	m.mu.RUnlock()
	for _, id := range dense {
		fn(id)
	}
}

// IDs returns a snapshot of all live entity IDs in creation order.
func (m *EntityManager) IDs() []EntityID {
	m.mu.RLock()
	defer m.mu.RUnlock()
	out := make([]EntityID, len(m.dense))
	copy(out, m.dense)
	return out
}

// entityManagerState is the serializable form of an EntityManager, capturing
// the full allocation state (live entities, next index, free list, and per-index
// versions) so a restore reproduces future Create/Destroy behavior exactly.
type entityManagerState struct {
	Dense    []EntityID `json:"dense"`
	Next     uint32     `json:"next"`
	Free     []uint32   `json:"free"`
	Versions []uint32   `json:"versions"`
}

func (m *EntityManager) snapshot() entityManagerState {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return entityManagerState{
		Dense:    append([]EntityID(nil), m.dense...),
		Next:     m.next,
		Free:     append([]uint32(nil), m.free...),
		Versions: append([]uint32(nil), m.versions...),
	}
}

func (m *EntityManager) restore(s entityManagerState) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.dense = append([]EntityID(nil), s.Dense...)
	m.next = s.Next
	m.free = append([]uint32(nil), s.Free...)
	m.versions = append([]uint32(nil), s.Versions...)
	m.sparse = make([]int32, len(m.versions))
	for pos, e := range m.dense {
		m.sparse[e.Index()] = int32(pos + 1)
	}
	m.count = len(m.dense)
}

// Len returns the number of live entities.
func (m *EntityManager) Len() int {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.count
}
