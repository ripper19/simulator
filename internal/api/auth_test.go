package api_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/ripper19/simulator/examples/counter"
	"github.com/ripper19/simulator/internal/api"
	"github.com/ripper19/simulator/internal/auth"
	"github.com/ripper19/simulator/internal/database"
	"github.com/ripper19/simulator/internal/persistence"
	"github.com/ripper19/simulator/internal/registry"
	"github.com/ripper19/simulator/internal/runner"
	"github.com/ripper19/simulator/internal/testutil"
	"github.com/ripper19/simulator/pkg/simulation"
)

func newAuthServer(t *testing.T) (*httptest.Server, *auth.Manager, *auth.Service) {
	t.Helper()
	pool, cleanup, ok := testutil.TestPool(t)
	if !ok {
		t.Skip("no database")
		return nil, nil, nil
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
	srv := api.New(mgr, reg, store, nil)
	tokens := auth.NewManager("test-secret", time.Hour, time.Hour)
	svc := auth.NewService(store, tokens)
	srv.SetAuth(svc, tokens)
	return httptest.NewServer(srv.Router()), tokens, svc
}

func doJSONAuth(t *testing.T, method, url string, body any, token string) (int, []byte) {
	t.Helper()
	req, err := newJSONRequest(method, url, body)
	if err != nil {
		t.Fatal(err)
	}
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	return resp.StatusCode, readAll(t, resp)
}

func TestAuthRegisterLoginAndOwnership(t *testing.T) {
	srv, _, _ := newAuthServer(t)
	defer srv.Close()

	code, _ := doJSON(t, "POST", srv.URL+"/api/v1/auth/register", map[string]any{"username": "alice", "password": "password123"})
	if code != http.StatusCreated {
		t.Fatalf("register status = %d, want 201", code)
	}
	doJSON(t, "POST", srv.URL+"/api/v1/auth/register", map[string]any{"username": "bob", "password": "password123"})

	code, data := doJSON(t, "POST", srv.URL+"/api/v1/auth/login", map[string]any{"username": "alice", "password": "password123"})
	if code != http.StatusOK {
		t.Fatalf("login status = %d: %s", code, data)
	}
	var pair auth.TokenPair
	if err := json.Unmarshal(data, &pair); err != nil {
		t.Fatal(err)
	}
	if pair.AccessToken == "" {
		t.Fatal("login did not return an access token")
	}

	// Protected endpoint without a token is rejected.
	if code, _ := doJSON(t, "GET", srv.URL+"/api/v1/simulations", nil); code != http.StatusUnauthorized {
		t.Fatalf("unauthenticated status = %d, want 401", code)
	}

	// Alice creates a simulation.
	code, data = doJSONAuth(t, "POST", srv.URL+"/api/v1/simulations", map[string]any{
		"model_id": "counter", "seed": 1,
	}, pair.AccessToken)
	if code != http.StatusCreated {
		t.Fatalf("create status = %d: %s", code, data)
	}
	var info persistence.SimulationInfo
	json.Unmarshal(data, &info)

	// Bob cannot access Alice's record.
	_, data = doJSON(t, "POST", srv.URL+"/api/v1/auth/login", map[string]any{"username": "bob", "password": "password123"})
	var bob auth.TokenPair
	json.Unmarshal(data, &bob)
	if code, _ := doJSONAuth(t, "GET", srv.URL+"/api/v1/simulations/"+info.ID, nil, bob.AccessToken); code != http.StatusNotFound {
		t.Fatalf("bob access status = %d, want 404", code)
	}
	// IDOR: Bob also cannot read state or control Alice's simulation.
	if code, _ := doJSONAuth(t, "GET", srv.URL+"/api/v1/simulations/"+info.ID+"/state", nil, bob.AccessToken); code != http.StatusNotFound {
		t.Fatalf("bob /state status = %d, want 404", code)
	}
	if code, _ := doJSONAuth(t, "POST", srv.URL+"/api/v1/simulations/"+info.ID+"/start", nil, bob.AccessToken); code != http.StatusNotFound {
		t.Fatalf("bob /start status = %d, want 404", code)
	}

	// Alice can access her own simulation.
	if code, _ := doJSONAuth(t, "GET", srv.URL+"/api/v1/simulations/"+info.ID, nil, pair.AccessToken); code != http.StatusOK {
		t.Fatalf("alice access status = %d, want 200", code)
	}
}

func TestAdminOnlyModelSync(t *testing.T) {
	srv, _, svc := newAuthServer(t)
	defer srv.Close()

	// Register a regular user.
	doJSON(t, "POST", srv.URL+"/api/v1/auth/register", map[string]any{"username": "user1", "password": "password1"})
	_, data := doJSON(t, "POST", srv.URL+"/api/v1/auth/login", map[string]any{"username": "user1", "password": "password1"})
	var userPair auth.TokenPair
	json.Unmarshal(data, &userPair)

	// A USER cannot sync models (admin-only).
	if code, _ := doJSONAuth(t, "POST", srv.URL+"/api/v1/models", nil, userPair.AccessToken); code != http.StatusForbidden {
		t.Fatalf("user sync status = %d, want 403", code)
	}

	// Bootstrap an admin and verify it can.
	if _, err := svc.BootstrapAdmin(context.Background(), "admin1", "password1"); err != nil {
		t.Fatal(err)
	}
	_, data = doJSON(t, "POST", srv.URL+"/api/v1/auth/login", map[string]any{"username": "admin1", "password": "password1"})
	var adminPair auth.TokenPair
	json.Unmarshal(data, &adminPair)
	if code, _ := doJSONAuth(t, "POST", srv.URL+"/api/v1/models", nil, adminPair.AccessToken); code != http.StatusOK {
		t.Fatalf("admin sync status = %d, want 200", code)
	}
}
