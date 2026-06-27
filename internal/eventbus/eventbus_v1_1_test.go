package eventbus_test

import (
	"testing"

	"github.com/goalos/goalos/internal/eventbus"
	"github.com/goalos/goalos/pkg/events"
)

// TestSubscribeForGoal_FiltersByGoalID 验证 per-goal 过滤（v0.1.0）。
func TestSubscribeForGoal_FiltersByGoalID(t *testing.T) {
	bus := eventbus.New()
	goal1Called := false
	goal2Called := false

	bus.SubscribeForGoal("goal-1", events.TypeActionCompleted, func(evt events.Event) error {
		goal1Called = true
		return nil
	})
	bus.SubscribeForGoal("goal-2", events.TypeActionCompleted, func(evt events.Event) error {
		goal2Called = true
		return nil
	})

	bus.Publish(events.Event{Type: events.TypeActionCompleted, GoalID: "goal-1", Seq: 1})

	if !goal1Called {
		t.Fatal("goal-1 handler was not called for goal-1 event")
	}
	if goal2Called {
		t.Fatal("goal-2 handler was incorrectly called for goal-1 event — per-goal filter failed")
	}

	goal1Called = false
	goal2Called = false
	bus.Publish(events.Event{Type: events.TypeActionCompleted, GoalID: "goal-2", Seq: 2})

	if goal1Called {
		t.Fatal("goal-1 handler was incorrectly called for goal-2 event")
	}
	if !goal2Called {
		t.Fatal("goal-2 handler was not called for goal-2 event")
	}
}

// TestSubscribeForGoal_EmptyGoalID_ReceivesAll 验证空 goalID 接收所有事件（向后兼容）。
func TestSubscribeForGoal_EmptyGoalID_ReceivesAll(t *testing.T) {
	bus := eventbus.New()
	allCalled := 0

	bus.SubscribeForGoal("", events.TypeActionCompleted, func(evt events.Event) error {
		allCalled++
		return nil
	})

	bus.Publish(events.Event{Type: events.TypeActionCompleted, GoalID: "goal-1", Seq: 1})
	bus.Publish(events.Event{Type: events.TypeActionCompleted, GoalID: "goal-2", Seq: 2})
	bus.Publish(events.Event{Type: events.TypeActionCompleted, GoalID: "goal-3", Seq: 3})

	if allCalled != 3 {
		t.Fatalf("empty-goalID handler should receive all events. got %d, want 3", allCalled)
	}
}
