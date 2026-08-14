package counter

import (
	"context"
	"testing"
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
