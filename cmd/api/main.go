// Command api runs the simulation platform's HTTP API service.
package main

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/ripper19/simulator/examples/counter"
	"github.com/ripper19/simulator/internal/api"
	"github.com/ripper19/simulator/internal/auth"
	"github.com/ripper19/simulator/internal/coord"
	"github.com/ripper19/simulator/internal/database"
	"github.com/ripper19/simulator/internal/metrics"
	"github.com/ripper19/simulator/internal/observability"
	"github.com/ripper19/simulator/internal/persistence"
	"github.com/ripper19/simulator/internal/registry"
	"github.com/ripper19/simulator/internal/runner"
	"github.com/ripper19/simulator/pkg/simulation"
)

func main() {
	logger := slog.New(slog.NewJSONHandler(os.Stdout, nil))

	url := os.Getenv("DATABASE_URL")
	if url == "" {
		logger.Error("DATABASE_URL is not set")
		os.Exit(1)
	}
	addr := os.Getenv("HTTP_ADDR")
	if addr == "" {
		addr = ":8080"
	}

	ctx := context.Background()
	pool, err := database.Open(ctx, url)
	if err != nil {
		logger.Error("connect database", "err", err)
		os.Exit(1)
	}
	defer pool.Close()
	if err := database.Migrate(ctx, pool); err != nil {
		logger.Error("migrate database", "err", err)
		os.Exit(1)
	}

	store := persistence.NewStore(pool)
	reg := registry.New()
	reg.Register((&counter.CounterWorld{}).Metadata(), func() simulation.Model {
		return &counter.CounterWorld{}
	})

	mgr := runner.NewManager(store, reg)
	mgr.SetMetrics(metrics.New(nil))
	server := api.New(mgr, reg, store, logger)

	if secret := os.Getenv("JWT_SECRET"); secret != "" {
		tokens := auth.NewManager(secret, 15*time.Minute, 24*time.Hour)
		svc := auth.NewService(store, tokens)
		server.SetAuth(svc, tokens)
		if uname, pwd := os.Getenv("ADMIN_USERNAME"), os.Getenv("ADMIN_PASSWORD"); uname != "" && pwd != "" {
			if _, err := svc.BootstrapAdmin(ctx, uname, pwd); err != nil {
				logger.Error("bootstrap admin", "err", err)
				os.Exit(1)
			}
			logger.Info("admin bootstrapped", "username", uname)
		}
	}
	if addr := os.Getenv("REDIS_ADDR"); addr != "" {
		if c, err := coord.NewRedis(addr, 0); err == nil {
			server.SetRedis(c)
			defer c.Close()
		}
	}
	if endpoint := os.Getenv("OTEL_EXPORTER_OTLP_ENDPOINT"); endpoint != "" {
		shutdown, err := observability.InitTracer(ctx, endpoint)
		if err != nil {
			logger.Error("tracing", "err", err)
			os.Exit(1)
		}
		defer shutdown(ctx)
	}

	srv := &http.Server{
		Addr:              addr,
		Handler:           server.Router(),
		ReadHeaderTimeout: 10 * time.Second,
	}

	go func() {
		logger.Info("api listening", "addr", addr)
		if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			logger.Error("server", "err", err)
			os.Exit(1)
		}
	}()

	stop := make(chan os.Signal, 1)
	signal.Notify(stop, syscall.SIGINT, syscall.SIGTERM)
	<-stop

	shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := srv.Shutdown(shutdownCtx); err != nil {
		logger.Error("shutdown", "err", err)
	}
	logger.Info("api stopped")
}
