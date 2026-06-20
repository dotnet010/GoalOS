package test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"github.com/goalos/goalos/internal/client"
	"github.com/goalos/goalos/internal/eventbus"
	"github.com/goalos/goalos/internal/governance"
	"github.com/goalos/goalos/internal/missionengine"
	"github.com/goalos/goalos/internal/scheduler"
	"github.com/goalos/goalos/internal/statestore"
	"github.com/goalos/goalos/pkg/events"
)

// TestE2EGoalLifecycle 验证完整的 Goal 生命周期端到端流程。
// W1 核心链路：Goal → PlanRequested → MissionGraph → ActionScheduled →
// Governance → ActionApproved → ActionCompleted → events.jsonl。
func TestE2EGoalLifecycle(t *testing.T) {
	dir := t.TempDir()
	bus := eventbus.New()
	store := statestore.New(dir)

	// Wire all modules (same as main.go)
	sched := scheduler.New(bus, store)
	sched.Start()

	gov := governance.New(bus)
	gov.Start()

	stub := &missionengine.StubAgent{}
	missionEng := missionengine.New(bus, stub)
	missionEng.Start()

	// Track received events (use counters, EventBus.Publish is synchronous)
	received := make(map[string]int)
	var mu sync.Mutex

	// Debug: trace all events
	bus.Subscribe(events.TypePlanRequested, func(evt events.Event) error {
		mu.Lock()
		received["PlanRequested"]++
		mu.Unlock()
		return nil
	})
	bus.Subscribe(events.TypeGoalCreated, func(evt events.Event) error {
		mu.Lock()
		received["GoalCreated"]++
		mu.Unlock()
		return nil
	})
	bus.Subscribe(events.TypeMissionGenerated, func(evt events.Event) error {
		mu.Lock()
		received["MissionGenerated"]++
		mu.Unlock()
		return nil
	})
	bus.Subscribe(events.TypeActionApproved, func(evt events.Event) error {
		mu.Lock()
		received["ActionApproved"]++
		mu.Unlock()
		return nil
	})
	bus.Subscribe(events.TypeActionCompleted, func(evt events.Event) error {
		mu.Lock()
		received["ActionCompleted"]++
		mu.Unlock()
		return nil
	})

	// Publish GoalCreated (synchronous — all downstream handlers run before Publish returns)
	goalID := "goal_e2e_001"
	bus.Publish(events.NewEvent(events.TypeGoalCreated, goalID, "daemon").WithPayload(map[string]interface{}{
		"title":       "开发CRM系统",
		"description": "面向中小企业的客户关系管理系统",
	}))

	// All events processed synchronously — verify immediately
	if received["GoalCreated"] != 1 {
		t.Errorf("expected 1 GoalCreated, got %d", received["GoalCreated"])
	}
	if received["MissionGenerated"] != 1 {
		t.Errorf("expected 1 MissionGenerated, got %d", received["MissionGenerated"])
	}
	if received["ActionApproved"] != 3 {
		t.Errorf("expected 3 ActionApproved (3 nodes), got %d", received["ActionApproved"])
	}
	if received["ActionCompleted"] != 3 {
		t.Errorf("expected 3 ActionCompleted, got %d", received["ActionCompleted"])
	}
}

// TestE2EHTTPAPI 验证 HTTP API 端到端。
func TestE2EHTTPAPI(t *testing.T) {
	dir := t.TempDir()
	bus := eventbus.New()
	store := statestore.New(dir)

	// Wire modules
	scheduler.New(bus, store).Start()
	governance.New(bus).Start()
	missionengine.New(bus, &missionengine.StubAgent{}).Start()

	// Create HTTP handler (same as main.go goalsHandler)
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/api/goals" {
			http.Error(w, `{"error":{"code":"INVALID_REQUEST"}}`, http.StatusBadRequest)
			return
		}
		var body struct{ Goal string `json:"goal"` }
		json.NewDecoder(r.Body).Decode(&body)
		if body.Goal == "" {
			http.Error(w, `{"error":{"code":"INVALID_REQUEST","message":"goal is required"}}`, http.StatusBadRequest)
			return
		}

		goalID := "goal_http_001"
		bus.Publish(events.NewEvent(events.TypeGoalCreated, goalID, "daemon").WithPayload(map[string]interface{}{
			"title": body.Goal,
		}))

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		json.NewEncoder(w).Encode(map[string]string{
			"goal_id": goalID,
			"status":  "created",
		})
	})

	srv := httptest.NewServer(handler)
	defer srv.Close()

	// Use client to call the HTTP API
	c := client.New(srv.URL)
	resp, err := c.CreateGoal("开发CRM系统")
	if err != nil {
		t.Fatalf("CreateGoal failed: %v", err)
	}
	if resp.GoalID != "goal_http_001" {
		t.Errorf("expected goal_http_001, got %s", resp.GoalID)
	}
	if !strings.Contains(resp.Status, "created") {
		t.Errorf("expected created status, got %s", resp.Status)
	}
}
