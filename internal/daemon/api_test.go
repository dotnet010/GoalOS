package daemon_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/goalos/goalos/internal/daemon"
)

func TestHandleHealth(t *testing.T) {
	h := daemon.NewHandler()
	req := httptest.NewRequest("GET", "/api/health", nil)
	w := httptest.NewRecorder()

	h.HandleHealth(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", w.Code)
	}
	var resp map[string]interface{}
	json.NewDecoder(w.Body).Decode(&resp)
	if resp["status"] != "ok" {
		t.Errorf("expected ok, got %v", resp["status"])
	}
}

func TestHandleCreateGoal(t *testing.T) {
	h := daemon.NewHandler()
	body := `{"goal":"开发CRM系统"}`
	req := httptest.NewRequest("POST", "/api/goals", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	h.HandleCreateGoal(w, req)

	if w.Code != http.StatusCreated {
		t.Errorf("expected 201, got %d: %s", w.Code, w.Body.String())
	}
	var resp map[string]interface{}
	json.NewDecoder(w.Body).Decode(&resp)
	if resp["goal_id"] == nil || resp["goal_id"] == "" {
		t.Errorf("expected goal_id, got %v", resp)
	}
}

func TestHandleCreateGoal_MissingGoal(t *testing.T) {
	h := daemon.NewHandler()
	body := `{"goal":""}`
	req := httptest.NewRequest("POST", "/api/goals", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	h.HandleCreateGoal(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", w.Code)
	}
}

func TestHandleGetGoal_NotFound(t *testing.T) {
	h := daemon.NewHandler()
	req := httptest.NewRequest("GET", "/api/goals/nonexistent", nil)
	req.SetPathValue("id", "nonexistent")
	w := httptest.NewRecorder()

	h.HandleGetGoal(w, req)

	if w.Code != http.StatusNotFound {
		t.Errorf("expected 404, got %d", w.Code)
	}
}

func TestHandleListGoals(t *testing.T) {
	h := daemon.NewHandler()
	// Create one goal first
	body := `{"goal":"test"}`
	req := httptest.NewRequest("POST", "/api/goals", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	h.HandleCreateGoal(w, req)

	// List
	req2 := httptest.NewRequest("GET", "/api/goals", nil)
	w2 := httptest.NewRecorder()
	h.HandleListGoals(w2, req2)

	if w2.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", w2.Code)
	}
	var goals []map[string]interface{}
	json.NewDecoder(w2.Body).Decode(&goals)
	if len(goals) < 1 {
		t.Errorf("expected at least 1 goal, got %d", len(goals))
	}
}

func TestHandlePauseResumeStop(t *testing.T) {
	h := daemon.NewHandler()

	// Create
	body := `{"goal":"test"}`
	req := httptest.NewRequest("POST", "/api/goals", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	h.HandleCreateGoal(w, req)
	var resp map[string]interface{}
	json.NewDecoder(w.Body).Decode(&resp)
	goalID := resp["goal_id"].(string)

	// Pause
	req2 := httptest.NewRequest("POST", "/api/goals/"+goalID+"/pause", nil)
	req2.SetPathValue("id", goalID)
	w2 := httptest.NewRecorder()
	h.HandlePauseGoal(w2, req2)
	if w2.Code != http.StatusOK {
		t.Errorf("pause: expected 200, got %d", w2.Code)
	}

	// Resume
	req3 := httptest.NewRequest("POST", "/api/goals/"+goalID+"/resume", nil)
	req3.SetPathValue("id", goalID)
	w3 := httptest.NewRecorder()
	h.HandleResumeGoal(w3, req3)
	if w3.Code != http.StatusOK {
		t.Errorf("resume: expected 200, got %d", w3.Code)
	}

	// Stop
	req4 := httptest.NewRequest("POST", "/api/goals/"+goalID+"/stop", nil)
	req4.SetPathValue("id", goalID)
	w4 := httptest.NewRecorder()
	h.HandleStopGoal(w4, req4)
	if w4.Code != http.StatusOK {
		t.Errorf("stop: expected 200, got %d", w4.Code)
	}
}
