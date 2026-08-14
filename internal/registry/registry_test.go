package registry_test

import (
	"context"
	"testing"

	"github.com/ripper19/simulator/examples/counter"
	"github.com/ripper19/simulator/internal/database"
	"github.com/ripper19/simulator/internal/persistence"
	"github.com/ripper19/simulator/internal/registry"
	"github.com/ripper19/simulator/internal/testutil"
	"github.com/ripper19/simulator/pkg/model"
	"github.com/ripper19/simulator/pkg/simulation"
)

func meta(id, version string) model.Metadata {
	return model.Metadata{ID: id, Name: id, Version: version, Mode: model.ModeTick}
}

func TestRegistryVersioning(t *testing.T) {
	r := registry.New()
	r.Register(meta("counter", "1.0.0"), nil)
	r.Register(meta("counter", "1.1.0"), nil)
	r.Register(meta("queue", "0.1.0"), nil)

	e, ok := r.Get("counter")
	if !ok || e.Info.Version != "1.1.0" {
		t.Fatalf("latest counter = %+v, want 1.1.0", e)
	}
	e, ok = r.GetVersion("counter", "1.0.0")
	if !ok || e.Info.Version != "1.0.0" {
		t.Fatalf("GetVersion(counter,1.0.0) = %+v", e)
	}
	if _, ok := r.GetVersion("counter", "9.9.9"); ok {
		t.Fatal("expected miss for unknown version")
	}
	if len(r.List()) != 3 {
		t.Fatalf("expected 3 models, got %d", len(r.List()))
	}
}

func TestRegistryLatestIsSemantic(t *testing.T) {
	r := registry.New()
	// Register a lower version first, then a higher one that would sort
	// lexicographically lower ("1.10.0" < "1.9.0" as strings).
	r.Register(meta("m", "1.9.0"), nil)
	r.Register(meta("m", "1.10.0"), nil)
	e, ok := r.Get("m")
	if !ok || e.Info.Version != "1.10.0" {
		t.Fatalf("latest = %+v, want 1.10.0 (semantic comparison)", e)
	}
	// And the reverse registration order.
	r2 := registry.New()
	r2.Register(meta("m", "1.10.0"), nil)
	r2.Register(meta("m", "1.9.0"), nil)
	e, ok = r2.Get("m")
	if !ok || e.Info.Version != "1.10.0" {
		t.Fatalf("latest = %+v, want 1.10.0", e)
	}
}

func TestRegistrySyncToStore(t *testing.T) {
	pool, cleanup, ok := testutil.TestPool(t)
	if !ok {
		return
	}
	defer cleanup()
	ctx := context.Background()
	if err := database.Migrate(ctx, pool); err != nil {
		t.Fatal(err)
	}
	store := persistence.NewStore(pool)

	r := registry.New()
	r.Register((&counter.CounterWorld{}).Metadata(), func() simulation.Model {
		return &counter.CounterWorld{}
	})
	if err := r.Sync(ctx, store); err != nil {
		t.Fatalf("sync: %v", err)
	}

	models, err := store.ListModels(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(models) != 1 {
		t.Fatalf("expected 1 synced model, got %d", len(models))
	}
	if models[0].ID != "counter" || models[0].Version != "1.0.0" || models[0].Mode != "tick" {
		t.Fatalf("unexpected synced model: %+v", models[0])
	}
}
