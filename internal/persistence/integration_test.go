package persistence

import (
	"context"
	"encoding/json"
	"reflect"
	"testing"

	"github.com/ripper19/simulator/internal/database"
	"github.com/ripper19/simulator/internal/testutil"
)

func newTestStore(t *testing.T) *Store {
	t.Helper()
	pool, cleanup, ok := testutil.TestPool(t)
	if !ok {
		t.Skip("no database")
		return nil
	}
	t.Cleanup(cleanup)
	if err := database.Migrate(context.Background(), pool); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	return NewStore(pool)
}

func TestModelCRUDAndVersioning(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	if err := s.UpsertModel(ctx, ModelInfo{ID: "counter", Name: "CounterWorld", Version: "1.0.0", Mode: "tick", Author: "a"}); err != nil {
		t.Fatal(err)
	}
	if err := s.UpsertModel(ctx, ModelInfo{ID: "counter", Name: "CounterWorld", Version: "1.1.0", Mode: "tick"}); err != nil {
		t.Fatal(err)
	}
	if err := s.UpsertModel(ctx, ModelInfo{ID: "queue", Name: "Queueing", Version: "0.1.0", Mode: "event"}); err != nil {
		t.Fatal(err)
	}

	models, err := s.ListModels(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(models) != 3 {
		t.Fatalf("expected 3 models, got %d", len(models))
	}

	m, err := s.GetModel(ctx, "counter", "1.0.0")
	if err != nil {
		t.Fatal(err)
	}
	if m.Name != "CounterWorld" || m.Mode != "tick" || m.Author != "a" {
		t.Fatalf("unexpected model: %+v", m)
	}

	versions, err := s.ListModelVersions(ctx, "counter")
	if err != nil {
		t.Fatal(err)
	}
	if len(versions) != 2 || versions[0].Version != "1.0.0" || versions[1].Version != "1.1.0" {
		t.Fatalf("unexpected versions: %+v", versions)
	}
}

func TestSimulationLifecycle(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	if err := s.UpsertModel(ctx, ModelInfo{ID: "counter", Version: "1.0.0", Mode: "tick"}); err != nil {
		t.Fatal(err)
	}
	si, err := s.CreateSimulation(ctx, SimulationInfo{
		ID: "sim-1", ModelID: "counter", ModelVersion: "1.0.0",
		Seed: 12345, Mode: "tick", Status: "created",
	})
	if err != nil {
		t.Fatal(err)
	}
	if si.Status != "created" {
		t.Fatalf("status = %q, want created", si.Status)
	}

	got, err := s.GetSimulation(ctx, "sim-1")
	if err != nil {
		t.Fatal(err)
	}
	if got.Seed != 12345 || got.ModelID != "counter" {
		t.Fatalf("unexpected simulation: %+v", got)
	}

	updated, err := s.UpdateSimulationStatus(ctx, "sim-1", "completed")
	if err != nil {
		t.Fatal(err)
	}
	if updated.Status != "completed" || updated.CompletedAt == nil {
		t.Fatalf("expected completed with completion time: %+v", updated)
	}

	list, err := s.ListSimulations(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(list) != 1 {
		t.Fatalf("expected 1 simulation, got %d", len(list))
	}
}

func TestSnapshotRoundTrip(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	if err := s.UpsertModel(ctx, ModelInfo{ID: "counter", Version: "1.0.0", Mode: "tick"}); err != nil {
		t.Fatal(err)
	}
	if _, err := s.CreateSimulation(ctx, SimulationInfo{
		ID: "sim-1", ModelID: "counter", ModelVersion: "1.0.0",
		Seed: 1, Mode: "tick", Status: "created",
	}); err != nil {
		t.Fatal(err)
	}

	data := []byte(`{"tick":10,"seed":1}`)
	if err := s.SaveSnapshot(ctx, SnapshotInfo{
		ID: "snap-1", SimulationID: "sim-1",
		SchemaVersion: 1, EngineVersion: "0.1.0",
		Data: data, Checksum: "deadbeef",
	}); err != nil {
		t.Fatal(err)
	}

	sn, err := s.GetSnapshot(ctx, "snap-1")
	if err != nil {
		t.Fatal(err)
	}
	// JSONB canonicalizes stored JSON (key order and whitespace), so compare
	// semantically rather than byte-for-byte.
	var gotData, wantData map[string]any
	if err := json.Unmarshal(sn.Data, &gotData); err != nil {
		t.Fatalf("unmarshal stored data: %v", err)
	}
	if err := json.Unmarshal(data, &wantData); err != nil {
		t.Fatalf("unmarshal expected data: %v", err)
	}
	if !reflect.DeepEqual(gotData, wantData) {
		t.Fatalf("snapshot data mismatch: %s vs %s", sn.Data, data)
	}
	if sn.Checksum != "deadbeef" {
		t.Fatalf("checksum = %q, want deadbeef", sn.Checksum)
	}

	list, err := s.ListSnapshots(ctx, "sim-1")
	if err != nil {
		t.Fatal(err)
	}
	if len(list) != 1 {
		t.Fatalf("expected 1 snapshot, got %d", len(list))
	}
}
