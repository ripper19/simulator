// Package api implements the HTTP API for the simulation platform.
package api

import (
	"log/slog"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/prometheus/client_golang/prometheus/promhttp"

	"github.com/ripper19/simulator/internal/auth"
	"github.com/ripper19/simulator/internal/coord"
	"github.com/ripper19/simulator/internal/observability"
	"github.com/ripper19/simulator/internal/persistence"
	"github.com/ripper19/simulator/internal/ratelimit"
	"github.com/ripper19/simulator/internal/registry"
	"github.com/ripper19/simulator/internal/runner"
)

// Server wires the API handlers to the runner, model registry, and store.
type Server struct {
	manager  *runner.Manager
	registry *registry.Registry
	store    *persistence.Store
	logger   *slog.Logger

	auth   *auth.Service
	tokens *auth.Manager
	redis  *coord.Redis
}

// New returns a Server.
func New(manager *runner.Manager, reg *registry.Registry, store *persistence.Store, logger *slog.Logger) *Server {
	if logger == nil {
		logger = slog.Default()
	}
	return &Server{manager: manager, registry: reg, store: store, logger: logger}
}

// SetAuth enables authentication with the given service and JWT manager.
func (s *Server) SetAuth(svc *auth.Service, tokens *auth.Manager) {
	s.auth = svc
	s.tokens = tokens
}

// SetRedis enables Redis-backed rate limiting.
func (s *Server) SetRedis(c *coord.Redis) { s.redis = c }

// Router builds the HTTP handler with all routes and middleware.
func (s *Server) Router() http.Handler {
	r := chi.NewRouter()
	r.Use(middleware.RequestID)
	r.Use(middleware.RealIP)
	r.Use(middleware.Recoverer)
	r.Use(middleware.Timeout(30 * time.Second))
	r.Use(observability.Middleware)
	r.Use(s.requestLogger())

	r.Get("/healthz", func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
	})
	r.Handle("/metrics", promhttp.Handler())

	r.Route("/api/v1", func(r chi.Router) {
		r.Route("/auth", func(r chi.Router) {
			r.Post("/register", s.register)
			r.Post("/login", s.login)
			r.Post("/refresh", s.refresh)
		})

		r.Group(func(r chi.Router) {
			if s.tokens != nil {
				r.Use(s.tokens.RequireAuth)
			}
			s.routes(r)
		})
	})
	return r
}

func (s *Server) routes(r chi.Router) {
	r.Route("/models", func(r chi.Router) {
		r.Get("/", s.listModels)
		r.Post("/", s.syncModels)
		r.Get("/{id}", s.getModel)
	})
	r.Route("/simulations", func(r chi.Router) {
		r.Post("/", s.createSimulation)
		r.Get("/", s.listSimulations)
		r.Route("/{id}", func(r chi.Router) {
			r.Get("/", s.getSimulation)
			r.Delete("/", s.deleteSimulation)
			r.Post("/start", s.action(s.manager.Start, http.StatusAccepted, "started"))
			r.Post("/pause", s.action(s.manager.Pause, http.StatusOK, "paused"))
			r.Post("/resume", s.action(s.manager.Resume, http.StatusOK, "resumed"))
			r.Post("/stop", s.action(s.manager.Stop, http.StatusOK, "stopped"))
			r.Post("/step", s.action(s.manager.Step, http.StatusOK, "stepped"))
			r.Post("/snapshot", s.snapshotSimulation)
			r.Post("/restore", s.restoreSimulation)
			r.Post("/replay", s.action(s.manager.Replay, http.StatusOK, "replayed"))
			r.Get("/state", s.simulationState)
			r.Get("/events", s.simulationEvents)
			r.Get("/metrics", s.simulationMetrics)
			r.Get("/stream", s.simulationStream)
		})
	})
}

// rateLimit applies Redis rate limiting to expensive endpoints.
func (s *Server) rateLimit(next http.Handler) http.Handler {
	if s.redis == nil {
		return next
	}
	return ratelimit.Middleware(s.redis, 60, time.Minute, nil)(next)
}

func (s *Server) requestLogger() func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			start := time.Now()
			ww := middleware.NewWrapResponseWriter(w, r.ProtoMajor)
			next.ServeHTTP(ww, r)
			s.logger.Info("request",
				"method", r.Method,
				"path", r.URL.Path,
				"status", ww.Status(),
				"duration", time.Since(start).String(),
				"request_id", middleware.GetReqID(r.Context()),
			)
		})
	}
}
