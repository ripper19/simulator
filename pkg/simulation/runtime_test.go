package simulation

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"
	"time"

	"github.com/ripper19/simulator/pkg/model"
)

// countingModel is a minimal tick model that increments an atomic counter each
// tick. The counter is atomic so tests can observe progress race-free.
type countingModel struct {
	steps atomic.Int64
}

func (m *countingModel) StepCount() int64 { return m.steps.Load() }

func (m *countingModel) Metadata() model.Metadata {
	return model.Metadata{ID: "counting", Name: "Counting", Version: "1", Mode: model.ModeTick}
}

func (m *countingModel) Initialize(ctx context.Context, w *World) error { return nil }

func (m *countingModel) Step(ctx context.Context, w *World) error {
	m.steps.Add(1)
	return nil
}

func TestRunCompletesAtMaxTicks(t *testing.T) {
	sim, err := New(context.Background(), Config{ID: "s", Seed: 1, Mode: model.ModeTick, MaxTicks: 100}, &countingModel{})
	if err != nil {
		t.Fatal(err)
	}
	if err := sim.Run(context.Background()); err != nil {
		t.Fatal(err)
	}
	if sim.State() != StateCompleted {
		t.Fatalf("state = %s, want completed", sim.State())
	}
	if sim.Ticks() != 100 {
		t.Fatalf("ticks = %d, want 100", sim.Ticks())
	}
}

func TestRunN(t *testing.T) {
	sim, err := New(context.Background(), Config{ID: "s", Seed: 1, Mode: model.ModeTick}, &countingModel{})
	if err != nil {
		t.Fatal(err)
	}
	if err := sim.RunN(context.Background(), 7); err != nil {
		t.Fatal(err)
	}
	if sim.Ticks() != 7 {
		t.Fatalf("ticks = %d, want 7", sim.Ticks())
	}
	if sim.State() != StateCompleted {
		t.Fatalf("state = %s, want completed", sim.State())
	}
}

func TestRunUntil(t *testing.T) {
	m := &countingModel{}
	sim, err := New(context.Background(), Config{ID: "s", Seed: 1, Mode: model.ModeTick}, m)
	if err != nil {
		t.Fatal(err)
	}
	err = sim.RunUntil(context.Background(), func() bool { return m.StepCount() >= 50 })
	if err != nil {
		t.Fatal(err)
	}
	if m.StepCount() != 50 {
		t.Fatalf("steps = %d, want 50", m.StepCount())
	}
}

func TestStepSingle(t *testing.T) {
	m := &countingModel{}
	sim, err := New(context.Background(), Config{ID: "s", Seed: 1, Mode: model.ModeTick}, m)
	if err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 10; i++ {
		if err := sim.Step(context.Background()); err != nil {
			t.Fatal(err)
		}
	}
	if m.StepCount() != 10 {
		t.Fatalf("steps = %d, want 10", m.StepCount())
	}
	if sim.Ticks() != 10 {
		t.Fatalf("ticks = %d, want 10", sim.Ticks())
	}
}

func waitFor(t *testing.T, cond func() bool, msg string) {
	t.Helper()
	deadline := time.Now().Add(3 * time.Second)
	for !cond() {
		if time.Now().After(deadline) {
			t.Fatalf("timeout waiting for %s", msg)
		}
		time.Sleep(time.Millisecond)
	}
}

func TestStartPauseResumeStop(t *testing.T) {
	m := &countingModel{}
	sim, err := New(context.Background(), Config{ID: "s", Seed: 1, Mode: model.ModeTick}, m)
	if err != nil {
		t.Fatal(err)
	}
	if err := sim.Start(context.Background()); err != nil {
		t.Fatal(err)
	}
	waitFor(t, func() bool { return m.StepCount() >= 5 }, "initial progress")

	if err := sim.Pause(); err != nil {
		t.Fatal(err)
	}
	if sim.State() != StatePaused {
		t.Fatalf("state = %s, want paused", sim.State())
	}
	// Pause takes effect at the next step boundary; allow one in-flight step to
	// settle, then assert no further advancement.
	time.Sleep(50 * time.Millisecond)
	at := m.StepCount()
	time.Sleep(100 * time.Millisecond)
	if m.StepCount() != at {
		t.Fatalf("steps advanced while paused: %d -> %d", at, m.StepCount())
	}

	if err := sim.Resume(); err != nil {
		t.Fatal(err)
	}
	waitFor(t, func() bool { return m.StepCount() > at }, "resume")

	if err := sim.Stop(); err != nil {
		t.Fatal(err)
	}
	if err := sim.Wait(); err != nil {
		t.Fatal(err)
	}
	if sim.State() != StateStopped {
		t.Fatalf("state = %s, want stopped", sim.State())
	}
}

func TestContextCancellation(t *testing.T) {
	m := &countingModel{}
	sim, err := New(context.Background(), Config{ID: "s", Seed: 1, Mode: model.ModeTick}, m)
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	if err := sim.Start(ctx); err != nil {
		t.Fatal(err)
	}
	waitFor(t, func() bool { return m.StepCount() >= 5 }, "initial progress")
	cancel()
	if err := sim.Wait(); err != nil {
		t.Fatal(err)
	}
	if sim.State() != StateStopped {
		t.Fatalf("state = %s, want stopped", sim.State())
	}
}

// failingModel returns an error on step, to verify failed state and error propagation.
type failingModel struct {
	countingModel
}

func (m *failingModel) Step(ctx context.Context, w *World) error {
	return errors.New("boom")
}

func TestRunErrorPropagation(t *testing.T) {
	sim, err := New(context.Background(), Config{ID: "s", Seed: 1, Mode: model.ModeTick}, &failingModel{})
	if err != nil {
		t.Fatal(err)
	}
	err = sim.Run(context.Background())
	if err == nil || err.Error() != "boom" {
		t.Fatalf("expected boom error, got %v", err)
	}
	if sim.State() != StateFailed {
		t.Fatalf("state = %s, want failed", sim.State())
	}
}

func TestBadMode(t *testing.T) {
	// A tick model requested in event mode must be rejected.
	_, err := New(context.Background(), Config{ID: "s", Seed: 1, Mode: model.ModeEvent}, &countingModel{})
	if err != ErrBadMode {
		t.Fatalf("expected ErrBadMode, got %v", err)
	}
}
