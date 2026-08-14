package queue

import (
	"context"
	"encoding/json"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/ripper19/simulator/internal/broker"
)

func TestRetryThenSuccess(t *testing.T) {
	b := broker.NewMemory()
	defer b.Close()
	q := New(b, nil, time.Millisecond)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	var mu sync.Mutex
	attempts := 0
	go func() {
		_ = q.Consume(ctx, func(ctx context.Context, job Job) error {
			mu.Lock()
			attempts++
			n := attempts
			mu.Unlock()
			if n < 3 {
				return errors.New("flaky")
			}
			return nil
		})
	}()

	if err := q.Enqueue(ctx, Job{ID: "j1", MaxAttempts: 5}); err != nil {
		t.Fatal(err)
	}

	deadline := time.Now().Add(2 * time.Second)
	for {
		mu.Lock()
		n := attempts
		mu.Unlock()
		if n >= 3 {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("job was not retried: attempts=%d", n)
		}
		time.Sleep(10 * time.Millisecond)
	}
}

func TestDeadLetter(t *testing.T) {
	b := broker.NewMemory()
	defer b.Close()
	q := New(b, nil, time.Millisecond)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	go func() {
		_ = q.Consume(ctx, func(ctx context.Context, job Job) error {
			return errors.New("always fails")
		})
	}()

	dlq := make(chan Job, 1)
	go func() {
		_ = b.Consume(ctx, QueueDLQ, func(ctx context.Context, d broker.Delivery) error {
			var job Job
			if err := json.Unmarshal(d.Body, &job); err != nil {
				return err
			}
			dlq <- job
			return d.Ack()
		})
	}()

	if err := q.Enqueue(ctx, Job{ID: "j2", MaxAttempts: 2}); err != nil {
		t.Fatal(err)
	}

	select {
	case job := <-dlq:
		if job.ID != "j2" {
			t.Fatalf("dlq job id = %q", job.ID)
		}
		if job.Attempt < 2 || job.LastError == "" {
			t.Fatalf("dlq job missing retry metadata: %+v", job)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("job was not dead-lettered")
	}
}
