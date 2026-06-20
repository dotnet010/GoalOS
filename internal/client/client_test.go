package client_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/goalos/goalos/internal/client"
)

func TestCreateGoal(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Errorf("expected POST, got %s", r.Method)
		}
		if r.URL.Path != "/api/goals" {
			t.Errorf("expected /api/goals, got %s", r.URL.Path)
		}
		var body struct{ Goal string `json:"goal"` }
		json.NewDecoder(r.Body).Decode(&body)
		if body.Goal != "开发CRM系统" {
			t.Errorf("expected '开发CRM系统', got %s", body.Goal)
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		w.Write([]byte(`{"goal_id":"goal_001","status":"created"}`))
	}))
	defer srv.Close()

	c := client.New(srv.URL)
	resp, err := c.CreateGoal("开发CRM系统")
	if err != nil {
		t.Fatalf("CreateGoal failed: %v", err)
	}
	if resp.GoalID != "goal_001" {
		t.Errorf("expected goal_001, got %s", resp.GoalID)
	}
}

func TestHealthCheck(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"status":"ok","pid":12345}`))
	}))
	defer srv.Close()

	c := client.New(srv.URL)
	healthy, err := c.Health()
	if err != nil {
		t.Fatalf("Health failed: %v", err)
	}
	if !healthy {
		t.Fatal("expected healthy daemon")
	}
}

func TestDaemonNotRunning(t *testing.T) {
	c := client.New("http://localhost:19999") // unused port
	_, err := c.Health()
	if err == nil {
		t.Fatal("expected error when daemon not running")
	}
}
