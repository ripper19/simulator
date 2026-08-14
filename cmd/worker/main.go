// Command worker runs the simulation worker service: it connects to the broker
// (and optionally Redis), registers itself, sends heartbeats, and executes
// simulation jobs dispatched by the platform.
package main

import (
	"context"
	"log/slog"
	"os"
	"os/signal"
	"syscall"

	"github.com/ripper19/simulator/examples/counter"
	"github.com/ripper19/simulator/internal/broker"
	"github.com/ripper19/simulator/internal/coord"
	"github.com/ripper19/simulator/internal/metrics"
	"github.com/ripper19/simulator/internal/queue"
	"github.com/ripper19/simulator/internal/registry"
	"github.com/ripper19/simulator/internal/workers"
	"github.com/ripper19/simulator/pkg/simulation"
)

func main() {
	logger := slog.New(slog.NewJSONHandler(os.Stdout, nil))

	brokerKind := envOr("BROKER", "memory") // memory | rabbitmq
	amqpURL := envOr("AMQP_URL", "amqp://guest:guest@127.0.0.1:5672/")
	redisAddr := os.Getenv("REDIS_ADDR") // optional
	workerID := os.Getenv("WORKER_ID")

	var br broker.Broker
	var err error
	switch brokerKind {
	case "rabbitmq":
		br, err = broker.NewRabbitMQ(amqpURL)
	default:
		br = broker.NewMemory()
	}
	if err != nil {
		logger.Error("broker", "err", err)
		os.Exit(1)
	}
	defer br.Close()

	var c *coord.Redis
	if redisAddr != "" {
		c, err = coord.NewRedis(redisAddr, 0)
		if err != nil {
			logger.Error("redis", "err", err)
			os.Exit(1)
		}
		defer c.Close()
	}

	reg := registry.New()
	reg.Register((&counter.CounterWorld{}).Metadata(), func() simulation.Model {
		return &counter.CounterWorld{}
	})

	q := queue.New(br, c, 0)
	met := metrics.New(nil)
	q.SetMetrics(met)
	svc := workers.NewService(workers.NewInfo(workerID, "0.1.0"), c, q, func(ctx context.Context, job queue.Job) error {
		return workers.RunJob(ctx, reg, q, job)
	})
	svc.SetMetrics(met)

	ctx, cancel := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer cancel()

	logger.Info("worker starting", "id", svc.Info().ID, "broker", brokerKind, "redis", redisAddr != "")
	if err := svc.Run(ctx); err != nil && ctx.Err() == nil {
		logger.Error("worker", "err", err)
		os.Exit(1)
	}
	logger.Info("worker stopped")
}

func envOr(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}
