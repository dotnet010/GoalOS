package scheduler_test

import (
	"testing"

	"github.com/goalos/goalos/internal/scheduler"
	"github.com/goalos/goalos/pkg/events"
)

func TestTransition_DraftToPlanned(t *testing.T) {
	next, emit, ok := scheduler.Transition(scheduler.StatusDraft, events.TypeMissionGenerated)
	if !ok {
		t.Fatal("transition not found")
	}
	if next != scheduler.StatusPlanned {
		t.Errorf("expected planned, got %s", next)
	}
	if len(emit) != 0 {
		t.Errorf("expected 0 emitted events, got %d", len(emit))
	}
}

func TestTransition_PlannedToRunning(t *testing.T) {
	next, emit, ok := scheduler.Transition(scheduler.StatusPlanned, events.TypeUserConfirmed)
	if !ok {
		t.Fatal("transition not found")
	}
	if next != scheduler.StatusRunning {
		t.Errorf("expected running, got %s", next)
	}
	if len(emit) != 1 || emit[0] != events.TypeActionScheduled {
		t.Errorf("expected ActionScheduled, got %v", emit)
	}
}

func TestTransition_RunningToPaused(t *testing.T) {
	next, emit, ok := scheduler.Transition(scheduler.StatusRunning, events.TypeGoalPauseRequested)
	if !ok {
		t.Fatal("transition not found")
	}
	if next != scheduler.StatusPaused {
		t.Errorf("expected paused, got %s", next)
	}
	if len(emit) != 1 || emit[0] != events.TypeGoalPaused {
		t.Errorf("expected GoalPaused, got %v", emit)
	}
}

func TestTransition_PausedToRunning(t *testing.T) {
	next, _, ok := scheduler.Transition(scheduler.StatusPaused, events.TypeGoalResumed)
	if !ok {
		t.Fatal("transition not found")
	}
	if next != scheduler.StatusRunning {
		t.Errorf("expected running, got %s", next)
	}
}

func TestTransition_RunningToCompleted(t *testing.T) {
	next, _, ok := scheduler.Transition(scheduler.StatusRunning, events.TypeGoalCompleted)
	if !ok {
		t.Fatal("transition not found")
	}
	if next != scheduler.StatusCompleted {
		t.Errorf("expected completed, got %s", next)
	}
}

func TestTransition_UnknownEvent(t *testing.T) {
	next, emit, ok := scheduler.Transition(scheduler.StatusRunning, "UnknownEvent")
	if ok {
		t.Fatal("expected transition not found")
	}
	if next != scheduler.StatusRunning {
		t.Errorf("state should not change on unknown event")
	}
	if emit != nil {
		t.Errorf("expected nil emit, got %v", emit)
	}
}

func TestUserVisible(t *testing.T) {
	tests := []struct {
		internal scheduler.GoalStatus
		expected string
	}{
		{scheduler.StatusDraft, "进行中"},
		{scheduler.StatusPlanned, "进行中"},
		{scheduler.StatusRunning, "进行中"},
		{scheduler.StatusRecovering, "进行中"},
		{scheduler.StatusPaused, "需要处理"},
		{scheduler.StatusFailed, "需要处理"},
		{scheduler.StatusCompleted, "已完成"},
	}
	for _, tt := range tests {
		got := scheduler.UserVisible(tt.internal)
		if got != tt.expected {
			t.Errorf("UserVisible(%s): expected %s, got %s", tt.internal, tt.expected, got)
		}
	}
}
