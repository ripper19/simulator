package api

import (
	"encoding/json"
	"errors"
	"net/http"

	"github.com/ripper19/simulator/internal/runner"
	"github.com/ripper19/simulator/pkg/simulation"
)

// apiError is a structured error returned to clients as JSON.
type apiError struct {
	Status  int    `json:"-"`
	Code    string `json:"code"`
	Message string `json:"message"`
}

func (e *apiError) Error() string { return e.Code + ": " + e.Message }

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func writeError(w http.ResponseWriter, err error) {
	var ae *apiError
	switch {
	case errors.As(err, &ae):
		writeJSON(w, ae.Status, map[string]any{"error": ae})
	case errors.Is(err, runner.ErrNotFound):
		writeJSON(w, http.StatusNotFound, map[string]any{"error": apiError{Status: http.StatusNotFound, Code: "not_found", Message: err.Error()}})
	case errors.Is(err, simulation.ErrAlreadyRunning) || errors.Is(err, simulation.ErrNotRunning):
		writeJSON(w, http.StatusConflict, map[string]any{"error": apiError{Status: http.StatusConflict, Code: "conflict", Message: err.Error()}})
	default:
		writeJSON(w, http.StatusInternalServerError, map[string]any{"error": apiError{Status: http.StatusInternalServerError, Code: "internal", Message: err.Error()}})
	}
}

func badRequest(code, msg string) *apiError {
	return &apiError{Status: http.StatusBadRequest, Code: code, Message: msg}
}

// decodeJSON decodes a JSON request body, enforcing a size limit.
func decodeJSON(r *http.Request, v any) error {
	r.Body = http.MaxBytesReader(nil, r.Body, 1<<20)
	dec := json.NewDecoder(r.Body)
	dec.DisallowUnknownFields()
	return dec.Decode(v)
}
