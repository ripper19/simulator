// Package metrics defines the Prometheus metrics for the platform's
// simulations, workers, snapshots, and queues.
package metrics

import (
	"github.com/prometheus/client_golang/prometheus"
)

// Metrics holds all platform metrics, registered on the given registerer.
type Metrics struct {
	SimulationStarted   prometheus.Counter
	SimulationCompleted prometheus.Counter
	SimulationFailed    prometheus.Counter
	SimulationActive    prometheus.Gauge
	TickDuration        prometheus.Histogram
	SnapshotDuration    prometheus.Histogram
	SnapshotSize        prometheus.Histogram
	WorkerJobsProcessed prometheus.Counter
	WorkerJobsFailed    prometheus.Counter
	WorkerActiveJobs    prometheus.Gauge
	QueueDepth          prometheus.Gauge
}

// New registers and returns a Metrics set. reg defaults to the global default
// registry when nil.
func New(reg prometheus.Registerer) *Metrics {
	if reg == nil {
		reg = prometheus.DefaultRegisterer
	}
	m := &Metrics{
		SimulationStarted: prometheus.NewCounter(prometheus.CounterOpts{
			Name: "simulation_started_total",
			Help: "Number of simulations started.",
		}),
		SimulationCompleted: prometheus.NewCounter(prometheus.CounterOpts{
			Name: "simulation_completed_total",
			Help: "Number of simulations that completed successfully.",
		}),
		SimulationFailed: prometheus.NewCounter(prometheus.CounterOpts{
			Name: "simulation_failed_total",
			Help: "Number of simulations that failed.",
		}),
		SimulationActive: prometheus.NewGauge(prometheus.GaugeOpts{
			Name: "simulation_active",
			Help: "Number of simulations currently running.",
		}),
		TickDuration: prometheus.NewHistogram(prometheus.HistogramOpts{
			Name:    "simulation_tick_duration_seconds",
			Help:    "Duration of a simulation tick.",
			Buckets: prometheus.ExponentialBuckets(0.0001, 2, 16),
		}),
		SnapshotDuration: prometheus.NewHistogram(prometheus.HistogramOpts{
			Name:    "snapshot_duration_seconds",
			Help:    "Duration of snapshot capture.",
			Buckets: prometheus.DefBuckets,
		}),
		SnapshotSize: prometheus.NewHistogram(prometheus.HistogramOpts{
			Name:    "snapshot_size_bytes",
			Help:    "Serialized snapshot size in bytes.",
			Buckets: prometheus.ExponentialBuckets(1024, 2, 16),
		}),
		WorkerJobsProcessed: prometheus.NewCounter(prometheus.CounterOpts{
			Name: "worker_jobs_processed_total",
			Help: "Number of jobs processed by workers.",
		}),
		WorkerJobsFailed: prometheus.NewCounter(prometheus.CounterOpts{
			Name: "worker_jobs_failed_total",
			Help: "Number of jobs that failed after retries.",
		}),
		WorkerActiveJobs: prometheus.NewGauge(prometheus.GaugeOpts{
			Name: "worker_active_jobs",
			Help: "Number of jobs currently being processed.",
		}),
		QueueDepth: prometheus.NewGauge(prometheus.GaugeOpts{
			Name: "queue_depth",
			Help: "Current number of queued jobs.",
		}),
	}
	reg.MustRegister(
		m.SimulationStarted, m.SimulationCompleted, m.SimulationFailed,
		m.SimulationActive, m.TickDuration, m.SnapshotDuration, m.SnapshotSize,
		m.WorkerJobsProcessed, m.WorkerJobsFailed, m.WorkerActiveJobs, m.QueueDepth,
	)
	return m
}
