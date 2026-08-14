// Package workers implements the worker service: a process that registers
// itself, sends heartbeats, and consumes simulation jobs from the broker,
// reporting results back.
package workers

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"os"
	"time"

	"github.com/ripper19/simulator/internal/coord"
	"github.com/ripper19/simulator/internal/metrics"
	"github.com/ripper19/simulator/internal/queue"
)

// Info is a worker's identity and metadata.
type Info struct {
	ID        string    `json:"id"`
	Hostname  string    `json:"hostname"`
	Version   string    `json:"version"`
	StartedAt time.Time `json:"started_at"`
}

// NewInfo builds worker identity, generating an ID when id is empty.
func NewInfo(id, version string) Info {
	hostname, _ := os.Hostname()
	if id == "" {
		id = hostname + "-" + newSuffix()
	}
	return Info{ID: id, Hostname: hostname, Version: version, StartedAt: time.Now()}
}

// ProcessFunc executes a single job and returns an error on failure.
type ProcessFunc func(ctx context.Context, job queue.Job) error

// Service runs the worker lifecycle: register, heartbeat, and consume jobs.
type Service struct {
	info              Info
	coord             *coord.Redis
	queue             *queue.Queue
	process           ProcessFunc
	met               *metrics.Metrics
	heartbeatInterval time.Duration
	heartbeatTTL      time.Duration
}

// NewService builds a worker service.
func NewService(info Info, c *coord.Redis, q *queue.Queue, process ProcessFunc) *Service {
	return &Service{
		info:              info,
		coord:             c,
		queue:             q,
		process:           process,
		heartbeatInterval: 5 * time.Second,
		heartbeatTTL:      15 * time.Second,
	}
}

// SetMetrics attaches a metrics set (may be nil to disable).
func (s *Service) SetMetrics(met *metrics.Metrics) { s.met = met }

// Run registers the worker, starts the heartbeat loop, and consumes jobs until
// ctx is cancelled.
func (s *Service) Run(ctx context.Context) error {
	if s.coord != nil {
		if err := s.register(ctx); err != nil {
			return err
		}
		go s.heartbeatLoop(ctx)
	}
	process := s.process
	if s.met != nil {
		met := s.met
		process = func(ctx context.Context, job queue.Job) error {
			met.WorkerActiveJobs.Inc()
			defer met.WorkerActiveJobs.Dec()
			if err := s.process(ctx, job); err != nil {
				met.WorkerJobsFailed.Inc()
				return err
			}
			met.WorkerJobsProcessed.Inc()
			return nil
		}
	}
	return s.queue.Consume(ctx, process)
}

// register records the worker's metadata and liveness in Redis.
func (s *Service) register(ctx context.Context) error {
	meta, err := json.Marshal(s.info)
	if err != nil {
		return err
	}
	if err := s.coord.Heartbeat(ctx, "worker:meta:"+s.info.ID, string(meta), 0); err != nil {
		return err
	}
	return s.coord.Heartbeat(ctx, "worker:heartbeat:"+s.info.ID, s.info.Version, s.heartbeatTTL)
}

// heartbeatLoop renews the liveness key until ctx is cancelled.
func (s *Service) heartbeatLoop(ctx context.Context) {
	ticker := time.NewTicker(s.heartbeatInterval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			_ = s.coord.Heartbeat(ctx, "worker:heartbeat:"+s.info.ID, s.info.Version, s.heartbeatTTL)
		}
	}
}

// Info returns the worker's identity.
func (s *Service) Info() Info { return s.info }

func newSuffix() string {
	var b [4]byte
	_, _ = rand.Read(b[:])
	return hex.EncodeToString(b[:])
}
