package api

import (
	"encoding/json"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"

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
	info, err := s.manager.Create(r.Context(), req)
	if err != nil {
		writeError(w, badRequest("create_failed", err.Error()))
		return
	}
	writeJSON(w, http.StatusCreated, info)
}

func (s *Server) listSimulations(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, s.manager.List())
}

func (s *Server) getSimulation(w http.ResponseWriter, r *http.Request) {
	info, ok := s.manager.Get(chi.URLParam(r, "id"))
	if !ok {
		writeError(w, &apiError{Status: http.StatusNotFound, Code: "not_found", Message: "simulation not found"})
		return
	}
	writeJSON(w, http.StatusOK, info)
}

func (s *Server) deleteSimulation(w http.ResponseWriter, r *http.Request) {
	if err := s.manager.Delete(r.Context(), chi.URLParam(r, "id")); err != nil {
		writeError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// ---- lifecycle ----

func (s *Server) startSimulation(w http.ResponseWriter, r *http.Request) {
	if err := s.manager.Start(r.Context(), chi.URLParam(r, "id")); err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusAccepted, map[string]string{"status": "started"})
}

func (s *Server) pauseSimulation(w http.ResponseWriter, r *http.Request) {
	if err := s.manager.Pause(r.Context(), chi.URLParam(r, "id")); err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "paused"})
}

func (s *Server) resumeSimulation(w http.ResponseWriter, r *http.Request) {
	if err := s.manager.Resume(r.Context(), chi.URLParam(r, "id")); err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "resumed"})
}

func (s *Server) stopSimulation(w http.ResponseWriter, r *http.Request) {
	if err := s.manager.Stop(r.Context(), chi.URLParam(r, "id")); err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "stopped"})
}

func (s *Server) stepSimulation(w http.ResponseWriter, r *http.Request) {
	if err := s.manager.Step(r.Context(), chi.URLParam(r, "id")); err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "stepped"})
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

func (s *Server) replaySimulation(w http.ResponseWriter, r *http.Request) {
	if err := s.manager.Replay(r.Context(), chi.URLParam(r, "id")); err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "replayed"})
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
