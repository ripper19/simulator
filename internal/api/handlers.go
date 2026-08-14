package api

import (
	"context"
	"encoding/json"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/ripper19/simulator/internal/auth"
	"github.com/ripper19/simulator/internal/persistence"
	"github.com/ripper19/simulator/internal/runner"
	"github.com/ripper19/simulator/pkg/simulation"
)

// ---- models ----

func (s *Server) listModels(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, s.registry.List())
}

func (s *Server) syncModels(w http.ResponseWriter, r *http.Request) {
	if err := s.registry.Sync(r.Context(), s.store); err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, s.registry.List())
}

func (s *Server) getModel(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	entry, ok := s.registry.Get(id)
	if !ok {
		writeError(w, &apiError{Status: http.StatusNotFound, Code: "not_found", Message: "model not found: " + id})
		return
	}
	writeJSON(w, http.StatusOK, entry.Info)
}

// ---- simulations ----

func (s *Server) createSimulation(w http.ResponseWriter, r *http.Request) {
	var req runner.CreateRequest
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, badRequest("invalid_request", err.Error()))
		return
	}
	if claims, ok := auth.FromContext(r.Context()); ok {
		req.OwnerID = claims.UserID
	}
	info, err := s.manager.Create(r.Context(), req)
	if err != nil {
		writeError(w, badRequest("create_failed", err.Error()))
		return
	}
	writeJSON(w, http.StatusCreated, info)
}

func (s *Server) listSimulations(w http.ResponseWriter, r *http.Request) {
	claims, _ := auth.FromContext(r.Context())
	var out []persistence.SimulationInfo
	for _, info := range s.manager.List() {
		if s.owns(claims, info) {
			out = append(out, info)
		}
	}
	writeJSON(w, http.StatusOK, out)
}

func (s *Server) getSimulation(w http.ResponseWriter, r *http.Request) {
	info, ok := s.manager.Get(chi.URLParam(r, "id"))
	if !ok {
		writeError(w, &apiError{Status: http.StatusNotFound, Code: "not_found", Message: "simulation not found"})
		return
	}
	if claims, _ := auth.FromContext(r.Context()); !s.owns(claims, info) {
		writeError(w, &apiError{Status: http.StatusNotFound, Code: "not_found", Message: "simulation not found"})
		return
	}
	writeJSON(w, http.StatusOK, info)
}

func (s *Server) deleteSimulation(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	if info, ok := s.manager.Get(id); ok {
		if claims, _ := auth.FromContext(r.Context()); !s.owns(claims, info) {
			writeError(w, &apiError{Status: http.StatusNotFound, Code: "not_found", Message: "simulation not found"})
			return
		}
	}
	if err := s.manager.Delete(r.Context(), id); err != nil {
		writeError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// owns reports whether the authenticated claims may access a simulation. ADMIN
// may access everything; otherwise the simulation must be owned by the user.
// When auth is disabled (nil claims), access is permitted.
func (s *Server) owns(claims *auth.Claims, info persistence.SimulationInfo) bool {
	if claims == nil || claims.Role == auth.RoleAdmin {
		return true
	}
	return info.OwnerID != nil && *info.OwnerID == claims.UserID
}

// ---- lifecycle ----

// action adapts a manager lifecycle operation to an HTTP handler.
func (s *Server) action(f func(context.Context, string) error, code int, status string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if err := f(r.Context(), chi.URLParam(r, "id")); err != nil {
			writeError(w, err)
			return
		}
		writeJSON(w, code, map[string]string{"status": status})
	}
}

func (s *Server) snapshotSimulation(w http.ResponseWriter, r *http.Request) {
	snap, err := s.manager.Snapshot(r.Context(), chi.URLParam(r, "id"))
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, snap)
}

func (s *Server) restoreSimulation(w http.ResponseWriter, r *http.Request) {
	var snap simulation.Snapshot
	if err := decodeJSON(r, &snap); err != nil {
		writeError(w, badRequest("invalid_request", err.Error()))
		return
	}
	if err := s.manager.Restore(r.Context(), chi.URLParam(r, "id"), &snap); err != nil {
		writeError(w, badRequest("restore_failed", err.Error()))
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "restored"})
}

// ---- state / events / metrics / stream ----

func (s *Server) simulationState(w http.ResponseWriter, r *http.Request) {
	st, err := s.manager.State(r.Context(), chi.URLParam(r, "id"))
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, st)
}

func (s *Server) simulationEvents(w http.ResponseWriter, r *http.Request) {
	sim, ok := s.manager.Simulation(chi.URLParam(r, "id"))
	if !ok {
		writeError(w, &apiError{Status: http.StatusNotFound, Code: "not_found", Message: "simulation not found"})
		return
	}
	writeJSON(w, http.StatusOK, sim.World().Events.PeekAll())
}

func (s *Server) simulationMetrics(w http.ResponseWriter, r *http.Request) {
	m, err := s.manager.Metrics(r.Context(), chi.URLParam(r, "id"))
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, m)
}

// simulationStream streams live progress as Server-Sent Events until the
// simulation reaches a terminal state or the client disconnects.
func (s *Server) simulationStream(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	sim, ok := s.manager.Simulation(id)
	if !ok {
		writeError(w, &apiError{Status: http.StatusNotFound, Code: "not_found", Message: "simulation not found"})
		return
	}

	fl, ok := w.(http.Flusher)
	if !ok {
		writeError(w, &apiError{Status: http.StatusInternalServerError, Code: "internal", Message: "streaming unsupported"})
		return
	}

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")

	ticker := time.NewTicker(200 * time.Millisecond)
	defer ticker.Stop()

	ctx := r.Context()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			st := sim.State()
			payload, _ := json.Marshal(map[string]any{
				"id":     id,
				"status": statusString(st),
				"tick":   sim.World().Clock.Tick(),
				"steps":  sim.Ticks(),
			})
			_, _ = w.Write([]byte("data: " + string(payload) + "\n\n"))
			fl.Flush()
			if st == simulation.StateCompleted || st == simulation.StateFailed || st == simulation.StateStopped {
				return
			}
		}
	}
}

func statusString(s simulation.State) string {
	switch s {
	case simulation.StateRunning:
		return "running"
	case simulation.StatePaused:
		return "paused"
	case simulation.StateCompleted:
		return "completed"
	case simulation.StateFailed:
		return "failed"
	case simulation.StateStopped:
		return "stopped"
	default:
		return "created"
	}
}

// ---- auth ----

func (s *Server) register(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Username string `json:"username"`
		Password string `json:"password"`
	}
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, badRequest("invalid_request", err.Error()))
		return
	}
	if req.Username == "" || req.Password == "" {
		writeError(w, badRequest("invalid_request", "username and password are required"))
		return
	}
	u, err := s.auth.Register(r.Context(), req.Username, req.Password)
	if err != nil {
		writeError(w, badRequest("register_failed", err.Error()))
		return
	}
	writeJSON(w, http.StatusCreated, u)
}

func (s *Server) login(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Username string `json:"username"`
		Password string `json:"password"`
	}
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, badRequest("invalid_request", err.Error()))
		return
	}
	pair, err := s.auth.Login(r.Context(), req.Username, req.Password)
	if err != nil {
		writeError(w, &apiError{Status: http.StatusUnauthorized, Code: "unauthorized", Message: "invalid credentials"})
		return
	}
	writeJSON(w, http.StatusOK, pair)
}

func (s *Server) refresh(w http.ResponseWriter, r *http.Request) {
	var req struct {
		RefreshToken string `json:"refresh_token"`
	}
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, badRequest("invalid_request", err.Error()))
		return
	}
	pair, err := s.auth.Refresh(r.Context(), req.RefreshToken)
	if err != nil {
		writeError(w, &apiError{Status: http.StatusUnauthorized, Code: "unauthorized", Message: "invalid refresh token"})
		return
	}
	writeJSON(w, http.StatusOK, pair)
}
