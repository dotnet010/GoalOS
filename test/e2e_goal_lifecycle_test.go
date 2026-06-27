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
	"github.com/goalos/goalos/internal/pluginrunner"
	"github.com/goalos/goalos/internal/scheduler"
	"github.com/goalos/goalos/internal/statestore"
	"github.com/goalos/goalos/pkg/events"
)

// TestE2EGoalLifecycle 验证完整的 Goal 生命周期端到端流程。
func TestE2EGoalLifecycle(t *testing.T) {
	dir := t.TempDir()
	bus := eventbus.New()
	store := statestore.New(dir)

	sched := scheduler.New(bus, store, scheduler.NewGoalAnchorTracker(20))
	sched.Start()

	gov := governance.New(bus, nil)
	gov.RegisterCapabilities("test-plugin", []string{"fs.read", "fs.write", "shell.execute", "browser.open", "browser.click"})
	gov.SetAutonomyLevel("autonomous") // v0.1.2: L3+ auto-approve in test
	gov.Start()

	stub := &missionengine.StubAgent{}
	missionEng := missionengine.New(bus, stub)
	missionEng.Start()

	runner := pluginrunner.New(bus, nil)
	runner.Start()

	received := make(map[string]int)
	var mu sync.Mutex

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
	bus.Subscribe(events.TypeActionFailed, func(evt events.Event) error {
		mu.Lock()
		received["ActionFailed"]++
		mu.Unlock()
		return nil
	})

	goalID := "goal_e2e_001"
	bus.Publish(events.NewEvent(events.TypeGoalCreated, goalID, "daemon").WithPayload(map[string]interface{}{
		"title":       "整理项目资料",
		"description": "测试无Plugin的fs.read动作",
	}))

	if received["GoalCreated"] != 1 {
		t.Errorf("expected 1 GoalCreated, got %d", received["GoalCreated"])
	}
	if received["MissionGenerated"] != 1 {
		t.Errorf("expected 1 MissionGenerated, got %d", received["MissionGenerated"])
	}
	if received["ActionApproved"] != 1 {
		t.Errorf("expected 1 ActionApproved (StubAgent now returns 1 focused node), got %d", received["ActionApproved"])
	}
	if received["ActionFailed"] != 1 {
		t.Errorf("expected 1 ActionFailed (no plugin binary for action type), got %d", received["ActionFailed"])
	}
}

// TestE2EHTTPAPI 验证 HTTP API 端到端。
func TestE2EHTTPAPI(t *testing.T) {
	dir := t.TempDir()
	bus := eventbus.New()
	store := statestore.New(dir)

	scheduler.New(bus, store, scheduler.NewGoalAnchorTracker(20)).Start()
	gov2 := governance.New(bus, nil)
	gov2.RegisterCapabilities("test-plugin", []string{"fs.read", "fs.write", "shell.execute", "browser.open", "browser.click"})
	gov2.Start()
	missionengine.New(bus, &missionengine.StubAgent{}).Start()
	pluginrunner.New(bus, nil).Start()

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
