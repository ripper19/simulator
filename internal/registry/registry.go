// Package registry holds the set of models available to the platform. Each
// entry carries durable metadata (persisted to PostgreSQL) and an optional
// factory that instantiates the model. The registry supports multiple versions
// of a model so a simulation can pin the exact version it executed with.
package registry

import (
	"context"
	"sort"
	"strconv"
	"strings"
	"sync"

	"github.com/ripper19/simulator/internal/persistence"
	"github.com/ripper19/simulator/pkg/model"
	"github.com/ripper19/simulator/pkg/simulation"
)

// Entry is a registered model: its durable metadata and an optional factory.
type Entry struct {
	Info    persistence.ModelInfo
	Factory func() simulation.Model
}

// Registry is an in-memory model registry. It is safe for concurrent use.
type Registry struct {
	mu      sync.RWMutex
	entries map[string]Entry // key: id + "@" + version
	latest  map[string]string
}

// New returns an empty registry.
func New() *Registry {
	return &Registry{
		entries: make(map[string]Entry),
		latest:  make(map[string]string),
	}
}

// Register adds a model from its runtime metadata and factory.
func (r *Registry) Register(meta model.Metadata, factory func() simulation.Model) {
	r.RegisterInfo(persistence.ModelInfo{
		ID:          meta.ID,
		Name:        meta.Name,
		Version:     meta.Version,
		Description: meta.Description,
		Mode:        meta.Mode.String(),
	}, factory)
}

// RegisterInfo adds a model with full durable metadata and an optional factory.
// Registering the same id@version replaces the previous entry. The "latest"
// version of an ID is tracked by semantic-version comparison.
func (r *Registry) RegisterInfo(info persistence.ModelInfo, factory func() simulation.Model) {
	r.mu.Lock()
	defer r.mu.Unlock()
	key := info.ID + "@" + info.Version
	r.entries[key] = Entry{Info: info, Factory: factory}
	if cur, ok := r.latest[info.ID]; !ok || compareVersion(info.Version, cur) >= 0 {
		r.latest[info.ID] = info.Version
	}
}

// Get returns the latest registered version of a model.
func (r *Registry) Get(id string) (Entry, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	version, ok := r.latest[id]
	if !ok {
		return Entry{}, false
	}
	return r.entries[id+"@"+version], true
}

// GetVersion returns a specific model version.
func (r *Registry) GetVersion(id, version string) (Entry, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	e, ok := r.entries[id+"@"+version]
	return e, ok
}

// List returns all registered models, sorted by ID then version.
func (r *Registry) List() []persistence.ModelInfo {
	r.mu.RLock()
	defer r.mu.RUnlock()
	out := make([]persistence.ModelInfo, 0, len(r.entries))
	for _, e := range r.entries {
		out = append(out, e.Info)
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].ID != out[j].ID {
			return out[i].ID < out[j].ID
		}
		return out[i].Version < out[j].Version
	})
	return out
}

// Sync upserts all registered model metadata into the store.
func (r *Registry) Sync(ctx context.Context, store *persistence.Store) error {
	for _, info := range r.List() {
		if err := store.UpsertModel(ctx, info); err != nil {
			return err
		}
	}
	return nil
}

// compareVersion compares two dot-separated numeric version strings (ignoring
// any pre-release suffix after the first '-' on a component). It returns -1, 0,
// or 1. This avoids the lexical trap where "1.10.0" sorts before "1.9.0".
func compareVersion(a, b string) int {
	pa := strings.Split(a, ".")
	pb := strings.Split(b, ".")
	n := len(pa)
	if len(pb) > n {
		n = len(pb)
	}
	for i := 0; i < n; i++ {
		x := componentInt(pa, i)
		y := componentInt(pb, i)
		if x < y {
			return -1
		}
		if x > y {
			return 1
		}
	}
	return 0
}

func componentInt(parts []string, i int) int {
	if i >= len(parts) {
		return 0
	}
	s := parts[i]
	if idx := strings.IndexAny(s, "-+"); idx >= 0 {
		s = s[:idx]
	}
	v, err := strconv.Atoi(s)
	if err != nil {
		return 0
	}
	return v
}
