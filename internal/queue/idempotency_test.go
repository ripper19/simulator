package queue

import (
	"context"
	"fmt"
	"os"
	"sync"
	"testing"
	"time"

	"github.com/ripper19/simulator/internal/broker"
	"github.com/ripper19/simulator/internal/coord"
)

// TestIdempotency verifies that duplicate delivery of the same job ID results in
// a single execution. Requires a live Redis (set REDIS_ADDR).
func TestIdempotency(t *testing.T) {
	addr := os.Getenv("REDIS_ADDR")
	if addr == "" {
		t.Skip("REDIS_ADDR not set; skipping idempotency test")
	}
	c, err := coord.NewRedis(addr, 0)
	if err != nil {
		t.Skipf("cannot connect to Redis: %v", err)
	}
	defer c.Close()

	b := broker.NewMemory()
	defer b.Close()
	q := New(b, c, time.Millisecond)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	jobID := fmt.Sprintf("idem-%d", time.Now().UnixNano())

	var mu sync.Mutex
	executions := 0
	go func() {
		_ = q.Consume(ctx, func(ctx context.Context, job Job) error {
			mu.Lock()
			executions++
			mu.Unlock()
			return nil
		})
	}()

	// Enqueue the same job twice (duplicate delivery).
	for i := 0; i < 2; i++ {
		if err := q.Enqueue(ctx, Job{ID: jobID, MaxAttempts: 1}); err != nil {
			t.Fatal(err)
		}
	}

	time.Sleep(300 * time.Millisecond)
	mu.Lock()
	n := executions
	mu.Unlock()
	if n != 1 {
		t.Fatalf("expected 1 execution for duplicate job, got %d", n)
	}
}
