package simulation

import (
	"context"
	"sync"
)

// scheduler executes a model's systems each tick. It layers systems into
// dependency-ordered levels (systems in the same level are independent and may
// run concurrently), and within a system it partitions the operated-on entities
// into deterministic, disjoint shards that run on a bounded worker pool.
//
// Determinism is preserved because:
//  1. Level ordering is a deterministic function of the systems' declared
//     read/write conflicts and their declared order.
//  2. Shard membership is a pure function of the entity index (index % workers),
//     independent of goroutine scheduling.
//  3. Each entity is written by at most one shard; the model's obligation is
//     that entity updates are self-contained (each entity's new state depends
//     only on its own prior state plus read-only shared state).
//
// The scheduler caches each system's entity set and invalidates it only when
// the World's structural revision changes (entity create/destroy or component
// add/remove), so a steady-state tick does not re-scan the component store.
type scheduler struct {
	workers int
	systems []System
	levels  [][]int // levels of system indices into systems
	mu      sync.Mutex
	cache   []entitySet // cache[i] for systems[i]
}

type entitySet struct {
	rev uint64
	ids []EntityID
}

// newScheduler builds a scheduler over systems with the given worker bound.
func newScheduler(systems []System, workers int) *scheduler {
	return &scheduler{
		workers: workers,
		systems: systems,
		levels:  dependencyLevels(systems),
		cache:   make([]entitySet, len(systems)),
	}
}

// dependencyLevels assigns each system to a level such that conflicting systems
// are in strictly increasing levels. The declared order is the tie-break, so
// the result is deterministic. Returns level 0 first as indices into systems.
func dependencyLevels(systems []System) [][]int {
	n := len(systems)
	levels := make([]int, n)
	maxLevel := 0
	for i := 0; i < n; i++ {
		lvl := 0
		for j := 0; j < i; j++ {
			if conflicts(systems[i], systems[j]) && levels[j] > lvl {
				lvl = levels[j]
			}
		}
		levels[i] = lvl + 1
		if levels[i] > maxLevel {
			maxLevel = levels[i]
		}
	}
	if maxLevel == 0 {
		return nil
	}
	out := make([][]int, maxLevel)
	for i := 0; i < n; i++ {
		out[levels[i]-1] = append(out[levels[i]-1], i)
	}
	return out
}

// conflicts reports whether two systems touch the same component with at least
// one of them writing it, requiring serialization.
func conflicts(a, b System) bool {
	return intersectsWrite(a.Writes(), b.Reads()) ||
		intersectsWrite(a.Writes(), b.Writes()) ||
		intersectsWrite(b.Writes(), a.Reads())
}

func intersectsWrite(writes, other []ComponentID) bool {
	for _, w := range writes {
		for _, o := range other {
			if w == o {
				return true
			}
		}
	}
	return false
}

// run executes the systems level by level.
func (s *scheduler) run(ctx context.Context, w *World) error {
	for _, level := range s.levels {
		if len(level) == 1 {
			if err := s.runSystem(ctx, w, level[0]); err != nil {
				return err
			}
			continue
		}
		if err := s.runLevelParallel(ctx, w, level); err != nil {
			return err
		}
	}
	return nil
}

// runLevelParallel runs several independent systems concurrently.
func (s *scheduler) runLevelParallel(ctx context.Context, w *World, level []int) error {
	return runConcurrent(ctx, len(level), func(ctx context.Context, i int) error {
		return s.runSystem(ctx, w, level[i])
	})
}

// runSystem partitions the system's entity set into shards and runs them on a
// bounded worker pool. A single shard takes the inline fast path.
func (s *scheduler) runSystem(ctx context.Context, w *World, idx int) error {
	sys := s.systems[idx]
	entities := s.entitySet(w, sys, idx)
	if len(entities) == 0 {
		return nil
	}
	workers := s.workers
	if serial, ok := sys.(SerialSystem); ok && serial.Serial() {
		workers = 1
	}
	shards := partition(entities, workers)
	if len(shards) == 1 {
		return sys.Run(ctx, w, shards[0])
	}

	sem := make(chan struct{}, s.workers)
	return runConcurrent(ctx, len(shards), func(ctx context.Context, i int) error {
		select {
		case sem <- struct{}{}:
		case <-ctx.Done():
			return ctx.Err()
		}
		defer func() { <-sem }()
		return sys.Run(ctx, w, shards[i])
	})
}

// runConcurrent runs n tasks concurrently, returning the first error (or nil).
func runConcurrent(ctx context.Context, n int, fn func(ctx context.Context, i int) error) error {
	var wg sync.WaitGroup
	errCh := make(chan error, n)
	for i := 0; i < n; i++ {
		i := i
		wg.Add(1)
		go func() {
			defer wg.Done()
			if err := fn(ctx, i); err != nil {
				errCh <- err
			}
		}()
	}
	wg.Wait()
	close(errCh)
	for err := range errCh {
		if err != nil {
			return err
		}
	}
	return nil
}

// entitySet returns the cached entity set for a system, recomputing it only
// when the World's structural revision has changed since the last computation.
func (s *scheduler) entitySet(w *World, sys System, idx int) []EntityID {
	rev := w.Revision()
	s.mu.Lock()
	if s.cache[idx].rev == rev {
		ids := s.cache[idx].ids
		s.mu.Unlock()
		return ids
	}
	s.mu.Unlock()

	ids := systemEntities(w, sys)
	s.mu.Lock()
	s.cache[idx] = entitySet{rev: rev, ids: ids}
	s.mu.Unlock()
	return ids
}

// systemEntities returns the entities a system operates on: the union of the
// entities holding any of its read or written components, or all entities when
// it declares no component access.
func systemEntities(w *World, sys System) []EntityID {
	reads := sys.Reads()
	writes := sys.Writes()
	if len(reads) == 0 && len(writes) == 0 {
		return w.Entities.IDs()
	}
	if len(reads)+len(writes) == 1 {
		if len(reads) == 1 {
			return w.Components.Entities(reads[0])
		}
		return w.Components.Entities(writes[0])
	}
	seen := make(map[EntityID]struct{})
	var out []EntityID
	add := func(es []EntityID) {
		for _, e := range es {
			if _, ok := seen[e]; !ok {
				seen[e] = struct{}{}
				out = append(out, e)
			}
		}
	}
	for _, id := range reads {
		add(w.Components.Entities(id))
	}
	for _, id := range writes {
		add(w.Components.Entities(id))
	}
	return out
}

// partition splits entities into exactly workers shards as contiguous ranges of
// the input order. Because the input order is the column's dense (insertion)
// order, each shard writes a contiguous region of the underlying SoA arrays,
// which is cache-friendly and avoids false sharing between shards. Shards are
// zero-copy sub-slices of entities; the system must treat them as read-only
// entity lists. Membership is deterministic for a stable input order.
func partition(entities []EntityID, workers int) [][]EntityID {
	if workers <= 1 {
		return [][]EntityID{entities}
	}
	n := len(entities)
	if workers > n {
		workers = n
	}
	shards := make([][]EntityID, workers)
	chunk := (n + workers - 1) / workers
	for i := 0; i < workers; i++ {
		start := i * chunk
		if start >= n {
			shards[i] = entities[n:n]
			continue
		}
		end := start + chunk
		if end > n {
			end = n
		}
		shards[i] = entities[start:end]
	}
	return shards
}
