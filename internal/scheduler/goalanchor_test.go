package scheduler_test

import (
	"testing"

	"github.com/goalos/goalos/internal/scheduler"
)

func TestGoalAnchor_Increment(t *testing.T) {
	g := scheduler.NewGoalAnchorTracker(3)

	if g.Increment("goal_001") {
		t.Error("should not trigger at count 1")
	}
	if g.Increment("goal_001") {
		t.Error("should not trigger at count 2")
	}
	if !g.Increment("goal_001") {
		t.Error("should trigger at count 3")
	}
}

func TestGoalAnchor_Reset(t *testing.T) {
	g := scheduler.NewGoalAnchorTracker(3)

	g.Increment("goal_001")
	g.Increment("goal_001")
	g.Reset("goal_001")

	if g.Count("goal_001") != 0 {
		t.Errorf("expected 0 after reset, got %d", g.Count("goal_001"))
	}
}

func TestGoalAnchor_IndependentGoals(t *testing.T) {
	g := scheduler.NewGoalAnchorTracker(3)

	g.Increment("goal_a")
	g.Increment("goal_a")
	// goal_b should be independent
	if g.Count("goal_b") != 0 {
		t.Errorf("goal_b should be 0, got %d", g.Count("goal_b"))
	}
	g.Increment("goal_b")
	if g.Count("goal_b") != 1 {
		t.Errorf("goal_b should be 1, got %d", g.Count("goal_b"))
	}
}
