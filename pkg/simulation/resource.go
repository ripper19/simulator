package simulation

import (
	"reflect"
	"sync"
)

// ResourceRegistry stores a single value per concrete type. Resources are
// singletons shared across the whole simulation (for example a shared spatial
// index, a configuration object, or a global counter). Unlike components,
// resources are not attached to any entity.
type ResourceRegistry struct {
	mu        sync.RWMutex
	resources map[reflect.Type]any
}

// NewResourceRegistry returns an empty resource registry.
func NewResourceRegistry() *ResourceRegistry {
	return &ResourceRegistry{resources: make(map[reflect.Type]any)}
}

// SetResource stores v under its concrete type, replacing any previous value.
func SetResource[T any](r *ResourceRegistry, v T) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.resources[reflect.TypeOf(v)] = v
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
