// Package api implements the HTTP API for the simulation platform.
package api

import (
	"context"
	"log/slog"
	"net/http"
	"strings"
	"sync/atomic"
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

	rateLimitPerMin int64
}

// New returns a Server.
func New(manager *runner.Manager, reg *registry.Registry, store *persistence.Store, logger *slog.Logger) *Server {
	if logger == nil {
		logger = slog.Default()
	}
	return &Server{manager: manager, registry: reg, store: store, logger: logger, rateLimitPerMin: 60}
}

// SetAuth enables authentication with the given service and JWT manager.
func (s *Server) SetAuth(svc *auth.Service, tokens *auth.Manager) {
	s.auth = svc
	s.tokens = tokens
}

// SetRedis enables Redis-backed rate limiting.
func (s *Server) SetRedis(c *coord.Redis) { s.redis = c }

// SetRateLimit configures the per-IP per-minute request limit (default 60).
func (s *Server) SetRateLimit(n int64) { s.rateLimitPerMin = n }

// Router builds the HTTP handler with all routes and middleware.
func (s *Server) Router() http.Handler {
	r := chi.NewRouter()
	r.Use(middleware.RequestID)
	r.Use(middleware.Recoverer)
	r.Use(s.timeout())
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
		if s.tokens != nil {
			r.With(s.tokens.RequireRole(auth.RoleAdmin)).Post("/", s.syncModels)
		} else {
			r.Post("/", s.syncModels)
		}
		r.Get("/{id}", s.getModel)
	})
	r.Route("/simulations", func(r chi.Router) {
		r.With(s.rateLimit).Post("/", s.createSimulation)
		r.Get("/", s.listSimulations)
		r.Route("/{id}", func(r chi.Router) {
			r.Use(s.ownership)
			r.Get("/", s.getSimulation)
			r.Delete("/", s.deleteSimulation)
			r.With(s.rateLimit).Post("/start", s.action(s.manager.Start, http.StatusAccepted, "started"))
			r.Post("/pause", s.action(s.manager.Pause, http.StatusOK, "paused"))
			r.Post("/resume", s.action(s.manager.Resume, http.StatusOK, "resumed"))
			r.Post("/stop", s.action(s.manager.Stop, http.StatusOK, "stopped"))
			r.Post("/step", s.action(s.manager.Step, http.StatusOK, "stepped"))
			r.With(s.rateLimit).Post("/snapshot", s.snapshotSimulation)
			r.With(s.rateLimit).Post("/restore", s.restoreSimulation)
			r.With(s.rateLimit).Post("/replay", s.action(s.manager.Replay, http.StatusOK, "replayed"))
			r.With(s.rateLimit).Get("/state", s.simulationState)
			r.Get("/events", s.simulationEvents)
			r.Get("/metrics", s.simulationMetrics)
			r.Get("/stream", s.simulationStream)
		})
	})
}

// ownership enforces per-user access to a simulation by ID for every
// /simulations/{id}/* route (IDOR protection).
func (s *Server) ownership(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if info, ok := s.manager.Get(chi.URLParam(r, "id")); ok {
			if claims, _ := auth.FromContext(r.Context()); !s.owns(claims, info) {
				writeError(w, &apiError{Status: http.StatusNotFound, Code: "not_found", Message: "simulation not found"})
				return
			}
		}
		next.ServeHTTP(w, r)
	})
}

// rateLimit applies Redis rate limiting to expensive endpoints.
func (s *Server) rateLimit(next http.Handler) http.Handler {
	if s.redis == nil {
		return next
	}
	return ratelimit.Middleware(s.redis, s.rateLimitPerMin, time.Minute, nil)(next)
}

// timeout applies a 30s request timeout returning 503, exempting long-lived
// SSE streams. A guard suppresses the "superfluous WriteHeader" noise when a
// handler races with the timeout.
func (s *Server) timeout() func(http.Handler) http.Handler {
	const dur = 30 * time.Second
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if strings.HasSuffix(r.URL.Path, "/stream") {
				next.ServeHTTP(w, r)
				return
			}
			var timedOut, wrote atomic.Bool
			tw := &timeoutWriter{ResponseWriter: w, timedOut: &timedOut, wrote: &wrote}
			ctx, cancel := context.WithTimeout(r.Context(), dur)
			defer cancel()
			stop := context.AfterFunc(ctx, func() {
				if wrote.Load() {
					return
				}
				timedOut.Store(true)
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusServiceUnavailable)
				_, _ = w.Write([]byte(`{"error":{"code":"timeout","message":"request timed out"}}`))
			})
			defer stop()
			next.ServeHTTP(tw, r.WithContext(ctx))
		})
	}
}

// timeoutWriter suppresses handler writes after a timeout has been reported.
type timeoutWriter struct {
	http.ResponseWriter
	timedOut *atomic.Bool
	wrote    *atomic.Bool
}

func (t *timeoutWriter) WriteHeader(status int) {
	if t.timedOut.Load() {
		return
	}
	t.wrote.Store(true)
	t.ResponseWriter.WriteHeader(status)
}

func (t *timeoutWriter) Write(b []byte) (int, error) {
	if t.timedOut.Load() {
		return 0, http.ErrHandlerTimeout
	}
	t.wrote.Store(true)
	return t.ResponseWriter.Write(b)
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
