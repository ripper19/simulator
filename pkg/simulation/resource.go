package simulation

import (
	"encoding/json"
	"fmt"
	"reflect"
	"sort"
	"sync"
)

// ResourceRegistry stores a single value per concrete type. Resources are
// singletons shared across the whole simulation (for example a shared spatial
// index, a configuration object, or a global counter). Unlike components,
// resources are not attached to any entity.
//
// The registry also tracks each stored value's Go type (by name) so that
// resources can be round-tripped through a snapshot.
type ResourceRegistry struct {
	mu        sync.RWMutex
	resources map[reflect.Type]any
	types     map[string]reflect.Type
}

// NewResourceRegistry returns an empty resource registry.
func NewResourceRegistry() *ResourceRegistry {
	return &ResourceRegistry{
		resources: make(map[reflect.Type]any),
		types:     make(map[string]reflect.Type),
	}
}

// SetResource stores v under its concrete type, replacing any previous value.
func SetResource[T any](r *ResourceRegistry, v T) {
	t := reflect.TypeOf(v)
	r.mu.Lock()
	defer r.mu.Unlock()
	r.resources[t] = v
	r.types[t.String()] = t
}

// GetResource returns the resource stored under type T, and whether it exists.
func GetResource[T any](r *ResourceRegistry) (T, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	v, ok := r.resources[reflect.TypeOf((*T)(nil)).Elem()]
	if !ok {
		var zero T
		return zero, false
	}
	return v.(T), true
}

// GetOrCreateResource returns the resource of type T, creating a zero value if
// none exists.
func GetOrCreateResource[T any](r *ResourceRegistry) T {
	if v, ok := GetResource[T](r); ok {
		return v
	}
	var zero T
	SetResource(r, zero)
	return zero
}

// snapshot serializes all resources deterministically (ordered by type name).
// Values must be JSON-marshalable (exported fields) to round-trip exactly.
func (r *ResourceRegistry) snapshot() ([]resourceSnapshot, error) {
	r.mu.RLock()
	type item struct {
		name string
		t    reflect.Type
		v    any
	}
	items := make([]item, 0, len(r.resources))
	for t, v := range r.resources {
		items = append(items, item{t.String(), t, v})
	}
	r.mu.RUnlock()

	sort.Slice(items, func(i, j int) bool { return items[i].name < items[j].name })

	out := make([]resourceSnapshot, 0, len(items))
	for _, it := range items {
		b, err := json.Marshal(it.v)
		if err != nil {
			return nil, fmt.Errorf("simulation: snapshot resource %q: %w", it.name, err)
		}
		out = append(out, resourceSnapshot{Type: it.name, Value: b})
	}
	return out, nil
}

// restore replaces the stored resource values from a snapshot. The resource's
// Go type must already be known to this registry (because the model set it
// during Initialize); an unknown type is an error.
func (r *ResourceRegistry) restore(snaps []resourceSnapshot) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.resources = make(map[reflect.Type]any, len(snaps))
	for _, sn := range snaps {
		t, ok := r.types[sn.Type]
		if !ok {
			return fmt.Errorf("simulation: snapshot contains unknown resource type %q", sn.Type)
		}
		v := reflect.New(t)
		if err := json.Unmarshal(sn.Value, v.Interface()); err != nil {
			return fmt.Errorf("simulation: decode resource %q: %w", sn.Type, err)
		}
		r.resources[t] = v.Elem().Interface()
	}
	return nil
}
