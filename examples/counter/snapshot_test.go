package counter

import (
	"context"
	"testing"

	"github.com/ripper19/simulator/pkg/model"
	"github.com/ripper19/simulator/pkg/simulation"
)

// TestSnapshotRestoreContinueDeterministic verifies that snapshotting a
// mid-run simulation and continuing (both in-place and into a fresh simulation
// via restore) produces the same final state as an uninterrupted run.
func TestSnapshotRestoreContinueDeterministic(t *testing.T) {
	const n = 5000
	const totalTicks = uint64(200)
	const splitAt = uint64(100)
	const seed = uint64(777)

	// Uninterrupted baseline.
	base, mb := newSim(t, seed, n, totalTicks, 1)
	if err := base.Run(context.Background()); err != nil {
		t.Fatal(err)
	}
	fpBase := mb.Fingerprint(base.World())

	// Snapshot mid-run, then continue in-place.
	cont, mc := newSim(t, seed, n, splitAt, 1)
	if err := cont.Run(context.Background()); err != nil {
		t.Fatal(err)
	}
	snap, err := cont.Snapshot()
	if err != nil {
		t.Fatal(err)
	}
	if err := cont.Run(context.Background()); err != nil { // another splitAt ticks
		t.Fatal(err)
	}
	if fp := mc.Fingerprint(cont.World()); fp != fpBase {
		t.Fatalf("in-place continuation diverged: %d != %d", fp, fpBase)
	}

	// Restore the snapshot into a fresh simulation and continue.
	rest, mr := newSim(t, seed, n, splitAt, 1)
	if err := rest.Restore(snap); err != nil {
		t.Fatal(err)
	}
	if err := rest.Run(context.Background()); err != nil {
		t.Fatal(err)
	}
	if fp := mr.Fingerprint(rest.World()); fp != fpBase {
		t.Fatalf("restore+continue diverged: %d != %d", fp, fpBase)
	}
}

// TestSnapshotCapturesModelConfig verifies the model's own configuration
// (N, IncrementMax) is captured in and restored from a snapshot.
func TestSnapshotCapturesModelConfig(t *testing.T) {
	m := &CounterWorld{N: 123, IncrementMax: 777}
	sim, err := simulation.New(context.Background(), simulation.Config{
		ID: "c", Seed: 5, Mode: model.ModeTick,
	}, m)
	if err != nil {
		t.Fatal(err)
	}
	snap, err := sim.Snapshot()
	if err != nil {
		t.Fatal(err)
	}
	if len(snap.ModelConfig) == 0 {
		t.Fatal("model config not captured in snapshot")
	}

	m2 := &CounterWorld{N: 1, IncrementMax: 1}
	sim2, err := simulation.New(context.Background(), simulation.Config{
		ID: "c", Seed: 5, Mode: model.ModeTick,
	}, m2)
	if err != nil {
		t.Fatal(err)
	}
	if err := sim2.Restore(snap); err != nil {
		t.Fatal(err)
	}
	if m2.N != 123 || m2.IncrementMax != 777 {
		t.Fatalf("model config not restored: N=%d IncrementMax=%d", m2.N, m2.IncrementMax)
	}
}
