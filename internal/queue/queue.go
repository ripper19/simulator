// Package queue implements the asynchronous job system on top of the broker:
// enqueue/dequeue of simulation jobs with retries, exponential backoff,
// dead-letter handling, and idempotency.
package queue

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/ripper19/simulator/internal/broker"
	"github.com/ripper19/simulator/internal/coord"
	"github.com/ripper19/simulator/internal/metrics"
)

// Queue names.
const (
	QueueJobs    = "simulator.jobs"
	QueueResults = "simulator.results"
	QueueDLQ     = "simulator.jobs.dlq"
)

// Job is a unit of work dispatched to workers.
type Job struct {
	ID           string          `json:"id"`
	SimulationID string          `json:"simulation_id"`
	Type         string          `json:"type"`
	Payload      json.RawMessage `json:"payload"`
	Attempt      int             `json:"attempt"`
	MaxAttempts  int             `json:"max_attempts"`
	LastError    string          `json:"last_error,omitempty"`
	CreatedAt    time.Time       `json:"created_at"`
}

// Result is the outcome of a job, published by a worker.
type Result struct {
	JobID        string          `json:"job_id"`
	SimulationID string          `json:"simulation_id"`
	Status       string          `json:"status"`
	Error        string          `json:"error,omitempty"`
	Snapshot     json.RawMessage `json:"snapshot,omitempty"`
	CompletedAt  time.Time       `json:"completed_at"`
}

// RunJobPayload describes how a worker should run a simulation. It is the
// payload of a "run_simulation" job and is engine-agnostic: the worker uses it
// to instantiate the model (from its compiled-in registry) and execute it.
type RunJobPayload struct {
	SimulationID string          `json:"simulation_id"`
	ModelID      string          `json:"model_id"`
	ModelVersion string          `json:"model_version"`
	Seed         uint64          `json:"seed"`
	Mode         string          `json:"mode"`
	MaxTicks     uint64          `json:"max_ticks"`
	MaxTime      float64         `json:"max_time"`
	Workers      int             `json:"workers"`
	Config       json.RawMessage `json:"config"`
}

// Queue enqueues and consumes jobs over a broker, with optional Redis
// coordination for idempotency.
type Queue struct {
	broker  broker.Broker
	coord   *coord.Redis
	backoff time.Duration
	met     *metrics.Metrics
}

// New returns a Queue. backoff is the base delay for exponential backoff.
func New(b broker.Broker, c *coord.Redis, backoff time.Duration) *Queue {
	if backoff <= 0 {
		backoff = time.Second
	}
	return &Queue{broker: b, coord: c, backoff: backoff}
}

// SetMetrics attaches a metrics set (may be nil to disable).
func (q *Queue) SetMetrics(met *metrics.Metrics) { q.met = met }

// Enqueue publishes a job to the main queue.
func (q *Queue) Enqueue(ctx context.Context, job Job) error {
	if job.MaxAttempts <= 0 {
		job.MaxAttempts = 1
	}
	data, err := json.Marshal(job)
	if err != nil {
		return err
	}
	if err := q.broker.Publish(ctx, QueueJobs, data); err != nil {
		return err
	}
	if q.met != nil {
		q.met.QueueDepth.Inc()
	}
	return nil
}

// PublishResult publishes a job result to the results queue.
func (q *Queue) PublishResult(ctx context.Context, r Result) error {
	data, err := json.Marshal(r)
	if err != nil {
		return err
	}
	return q.broker.Publish(ctx, QueueResults, data)
}

// Consume delivers jobs to handler, applying idempotency, retries, backoff, and
// dead-lettering. It blocks until ctx is cancelled.
//
// Idempotency: the job's processed key is claimed before execution and, on
// failure, released again so the retry can re-claim it (otherwise a failed
// first attempt would swallow every retry). On success the claim persists as
// the 24h processed marker.
func (q *Queue) Consume(ctx context.Context, handler func(ctx context.Context, job Job) error) error {
	return q.broker.Consume(ctx, QueueJobs, func(ctx context.Context, d broker.Delivery) error {
		var job Job
		if err := json.Unmarshal(d.Body, &job); err != nil {
			_ = d.Ack()
			return nil
		}

		key := "job:processed:" + job.ID
		if q.coord != nil {
			claimed, err := q.coord.Claim(ctx, key, "1", 24*time.Hour)
			if err == nil && !claimed {
				_ = d.Ack()
				return nil
			}
		}

		if err := handler(ctx, job); err != nil {
			if q.coord != nil {
				_ = q.coord.Release(ctx, key)
			}
			q.done()
			q.handleFailure(ctx, d, job, err)
			return nil
		}
		q.done()
		return d.Ack()
	})
}

func (q *Queue) done() {
	if q.met != nil {
		q.met.QueueDepth.Dec()
	}
}

// handleFailure acks the original message and either re-publishes with a
// backing-off retry or dead-letters the job.
func (q *Queue) handleFailure(ctx context.Context, d broker.Delivery, job Job, err error) {
	_ = d.Ack()
	job.LastError = err.Error()
	if job.Attempt >= job.MaxAttempts {
		_ = q.publish(ctx, QueueDLQ, job)
		return
	}
	job.Attempt++
	delay := q.backoffFor(job.Attempt)
	go func() {
		select {
		case <-ctx.Done():
			return
		case <-time.After(delay):
		}
		_ = q.publish(context.Background(), QueueJobs, job)
	}()
}

// backoffFor returns exponential backoff: base * 2^(attempt-1).
func (q *Queue) backoffFor(attempt int) time.Duration {
	d := q.backoff
	for i := 1; i < attempt; i++ {
		d *= 2
	}
	return d
}

func (q *Queue) publish(ctx context.Context, queue string, job Job) error {
	data, err := json.Marshal(job)
	if err != nil {
		return err
	}
	if err := q.broker.Publish(ctx, queue, data); err != nil {
		return fmt.Errorf("queue: publish %s: %w", queue, err)
	}
	return nil
}
