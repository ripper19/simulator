package api_test

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/ripper19/simulator/examples/counter"
	"github.com/ripper19/simulator/internal/api"
	"github.com/ripper19/simulator/internal/database"
	"github.com/ripper19/simulator/internal/persistence"
	"github.com/ripper19/simulator/internal/registry"
	"github.com/ripper19/simulator/internal/runner"
	"github.com/ripper19/simulator/internal/testutil"
	"github.com/ripper19/simulator/pkg/simulation"
)

func newTestServer(t *testing.T) *httptest.Server {
	t.Helper()
	pool, cleanup, ok := testutil.TestPool(t)
	if !ok {
		t.Skip("no database")
		return nil
	}
	t.Cleanup(cleanup)
	if err := database.Migrate(context.Background(), pool); err != nil {
		t.Fatal(err)
	}
	store := persistence.NewStore(pool)
	reg := registry.New()
	reg.Register((&counter.CounterWorld{}).Metadata(), func() simulation.Model {
		return &counter.CounterWorld{}
	})
	mgr := runner.NewManager(store, reg)
	return httptest.NewServer(api.New(mgr, reg, store, nil).Router())
}

func doJSON(t *testing.T, method, url string, body any) (int, []byte) {
	t.Helper()
	var r io.Reader
	if body != nil {
		b, err := json.Marshal(body)
		if err != nil {
			t.Fatal(err)
		}
		r = bytes.NewReader(b)
	}
	req, err := http.NewRequest(method, url, r)
	if err != nil {
		t.Fatal(err)
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	data, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatal(err)
	}
	return resp.StatusCode, data
}

func TestModelsList(t *testing.T) {
	srv := newTestServer(t)
	defer srv.Close()

	code, data := doJSON(t, "GET", srv.URL+"/api/v1/models", nil)
	if code != http.StatusOK {
		t.Fatalf("status %d: %s", code, data)
	}
	var models []persistence.ModelInfo
	if err := json.Unmarshal(data, &models); err != nil {
		t.Fatal(err)
	}
	if len(models) != 1 || models[0].ID != "counter" {
		t.Fatalf("unexpected models: %+v", models)
	}
}

func TestCreateStepAndState(t *testing.T) {
	srv := newTestServer(t)
	defer srv.Close()

	code, data := doJSON(t, "POST", srv.URL+"/api/v1/simulations", map[string]any{
		"model_id": "counter",
		"seed":     42,
		"config":   map[string]any{"n": 100},
	})
	if code != http.StatusCreated {
		t.Fatalf("create status %d: %s", code, data)
	}
	var info persistence.SimulationInfo
	if err := json.Unmarshal(data, &info); err != nil {
		t.Fatal(err)
	}
	if info.ID == "" || info.Status != "created" {
		t.Fatalf("unexpected record: %+v", info)
	}

	code, data = doJSON(t, "POST", srv.URL+"/api/v1/simulations/"+info.ID+"/step", nil)
	if code != http.StatusOK {
		t.Fatalf("step status %d: %s", code, data)
	}

	code, data = doJSON(t, "GET", srv.URL+"/api/v1/simulations/"+info.ID+"/state", nil)
	if code != http.StatusOK {
		t.Fatalf("state status %d: %s", code, data)
	}
	var st runner.State
	if err := json.Unmarshal(data, &st); err != nil {
		t.Fatal(err)
	}
	if st.Steps != 1 || st.Tick != 1 {
		t.Fatalf("unexpected state: %+v", st)
	}

	code, data = doJSON(t, "GET", srv.URL+"/api/v1/simulations/"+info.ID+"/metrics", nil)
	if code != http.StatusOK {
		t.Fatalf("metrics status %d: %s", code, data)
	}
	var m runner.Metrics
	if err := json.Unmarshal(data, &m); err != nil {
		t.Fatal(err)
	}
	if m.Entities != 100 {
		t.Fatalf("unexpected metrics: %+v", m)
	}
}

func TestStartToCompletionAndSnapshot(t *testing.T) {
	srv := newTestServer(t)
	defer srv.Close()

	code, data := doJSON(t, "POST", srv.URL+"/api/v1/simulations", map[string]any{
		"model_id":  "counter",
		"seed":      7,
		"max_ticks": 50,
		"config":    map[string]any{"n": 1000},
	})
	if code != http.StatusCreated {
		t.Fatalf("create status %d: %s", code, data)
	}
	var info persistence.SimulationInfo
	json.Unmarshal(data, &info)

	code, data = doJSON(t, "POST", srv.URL+"/api/v1/simulations/"+info.ID+"/start", nil)
	if code != http.StatusAccepted {
		t.Fatalf("start status %d: %s", code, data)
	}

	deadline := time.Now().Add(5 * time.Second)
	for {
		code, data = doJSON(t, "GET", srv.URL+"/api/v1/simulations/"+info.ID+"/state", nil)
		var st runner.State
		json.Unmarshal(data, &st)
		if st.Status == "completed" || st.Status == "failed" {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("simulation did not complete: %+v", st)
		}
		time.Sleep(20 * time.Millisecond)
	}

	code, data = doJSON(t, "POST", srv.URL+"/api/v1/simulations/"+info.ID+"/snapshot", nil)
	if code != http.StatusOK {
		t.Fatalf("snapshot status %d: %s", code, data)
	}
	var snap simulation.Snapshot
	if err := json.Unmarshal(data, &snap); err != nil {
		t.Fatal(err)
	}
	if err := snap.Validate(); err != nil {
		t.Fatalf("snapshot invalid: %v", err)
	}
}

func TestNotFound(t *testing.T) {
	srv := newTestServer(t)
	defer srv.Close()

	code, _ := doJSON(t, "GET", srv.URL+"/api/v1/simulations/missing", nil)
	if code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404", code)
	}
}

func TestHealth(t *testing.T) {
	srv := newTestServer(t)
	defer srv.Close()

	code, _ := doJSON(t, "GET", srv.URL+"/healthz", nil)
	if code != http.StatusOK {
		t.Fatalf("health status = %d, want 200", code)
	}
}
