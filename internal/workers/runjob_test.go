package workers_test

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/ripper19/simulator/examples/counter"
	"github.com/ripper19/simulator/internal/broker"
	"github.com/ripper19/simulator/internal/queue"
	"github.com/ripper19/simulator/internal/registry"
	"github.com/ripper19/simulator/internal/workers"
	"github.com/ripper19/simulator/pkg/simulation"
)

func newRegistry() *registry.Registry {
	reg := registry.New()
	reg.Register((&counter.CounterWorld{}).Metadata(), func() simulation.Model {
		return &counter.CounterWorld{}
	})
	return reg
}

func TestDistributedExecution(t *testing.T) {
	b := broker.NewMemory()
	defer b.Close()
	q := queue.New(b, nil, time.Millisecond)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	reg := newRegistry()

	// Start the worker consume loop.
	go func() {
		_ = q.Consume(ctx, func(ctx context.Context, job queue.Job) error {
			return workers.RunJob(ctx, reg, q, job)
		})
	}()

	// Consume results.
	results := make(chan queue.Result, 1)
	go func() {
		_ = b.Consume(ctx, queue.QueueResults, func(ctx context.Context, d broker.Delivery) error {
			var r queue.Result
			if err := json.Unmarshal(d.Body, &r); err != nil {
				return err
			}
			results <- r
			return d.Ack()
		})
	}()

	payload, err := json.Marshal(queue.RunJobPayload{
		SimulationID: "sim-1",
		ModelID:      "counter",
		Seed:         42,
		Mode:         "tick",
		MaxTicks:     100,
		Config:       json.RawMessage(`{"n":1000}`),
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := q.Enqueue(ctx, queue.Job{ID: "job-1", Type: "run_simulation", Payload: payload, MaxAttempts: 1}); err != nil {
		t.Fatal(err)
	}

	select {
	case r := <-results:
		if r.SimulationID != "sim-1" || r.Status != "completed" {
			t.Fatalf("unexpected result: %+v", r)
		}
		if len(r.Snapshot) == 0 {
			t.Fatal("result missing snapshot")
		}
	case <-time.After(5 * time.Second):
		t.Fatal("timed out waiting for result")
	}
}

func TestDistributedExecutionDeterminism(t *testing.T) {
	run := func(seed uint64) string {
		b := broker.NewMemory()
		defer b.Close()
		q := queue.New(b, nil, time.Millisecond)
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		go func() {
			_ = q.Consume(ctx, func(ctx context.Context, job queue.Job) error {
				return workers.RunJob(ctx, newRegistry(), q, job)
			})
		}()

		results := make(chan queue.Result, 1)
		go func() {
			_ = b.Consume(ctx, queue.QueueResults, func(ctx context.Context, d broker.Delivery) error {
				var r queue.Result
				_ = json.Unmarshal(d.Body, &r)
				results <- r
				return d.Ack()
			})
		}()

		payload, _ := json.Marshal(queue.RunJobPayload{
			SimulationID: "sim", ModelID: "counter", Seed: seed, Mode: "tick",
			MaxTicks: 50, Config: json.RawMessage(`{"n":2000}`),
		})
		_ = q.Enqueue(ctx, queue.Job{ID: "j", Type: "run_simulation", Payload: payload, MaxAttempts: 1})
		r := <-results
		return string(r.Snapshot)
	}

	a := run(12345)
	b := run(12345)
	if a != b {
		t.Fatal("distributed execution is not deterministic across runs")
	}
}
