// W1 核心链路集成测试。
// 验证：GoalCreated → events.jsonl → State Store Replay → 完整事件流闭环。
package test

import (
	"os"
	"testing"

	"github.com/goalos/goalos/internal/eventbus"
	"github.com/goalos/goalos/internal/statestore"
	"github.com/goalos/goalos/pkg/events"
)

// TestCoreChain 验证 W1 核心链路：
// 用户 Goal → GoalCreated 事件 → events.jsonl → State Store Replay → 事件完整。
func TestCoreChain(t *testing.T) {
	dir := t.TempDir()
	bus := eventbus.New()
	store := statestore.New(dir)

	goalID := "goal_chain_001"
	goalText := "开发一个CRM系统"

	// Step 1: Simulate user creating a Goal
	evt := events.Event{
		Seq:     1,
		Type:    events.TypeGoalCreated,
		GoalID:  goalID,
		Source:  "daemon",
		Version: "1.0",
		Payload: map[string]interface{}{
			"title":       goalText,
			"description": goalText,
		},
	}

	// Step 2: Publish event — Event Bus delivers to subscribers
	goalCreated := false
	bus.Subscribe(events.TypeGoalCreated, func(e events.Event) error {
		if e.GoalID == goalID {
			goalCreated = true
		}
		return nil
	})
	bus.Publish(evt)
	if !goalCreated {
		t.Fatal("GoalCreated event was not delivered to subscriber")
	}

	// Step 3: Persist to events.jsonl
	if err := store.Append(goalID, evt); err != nil {
		t.Fatalf("Append failed: %v", err)
	}

	// Step 4: Replay — verify event is recoverable
	replayed, err := store.Replay(goalID, 0)
	if err != nil {
		t.Fatalf("Replay failed: %v", err)
	}
	if len(replayed) != 1 {
		t.Fatalf("expected 1 replayed event, got %d", len(replayed))
	}

	// Step 5: State snapshot save and load
	state := &statestore.GoalState{
		GoalID:         goalID,
		InternalState:  "draft",
		LastAppliedSeq: 1,
	}
	if err := store.SaveState(goalID, state); err != nil {
		t.Fatalf("SaveState failed: %v", err)
	}

	loaded, err := store.LoadState(goalID)
	if err != nil {
		t.Fatalf("LoadState failed: %v", err)
	}
	if loaded.InternalState != "draft" {
		t.Errorf("expected draft, got %s", loaded.InternalState)
	}
	if loaded.LastAppliedSeq != 1 {
		t.Errorf("expected seq 1, got %d", loaded.LastAppliedSeq)
	}

	if err := os.RemoveAll(dir); err != nil {
		t.Logf("cleanup warning: %v", err)
	}
}

// TestCoreChainMultipleEvents 验证多事件流。
func TestCoreChainMultipleEvents(t *testing.T) {
	dir := t.TempDir()
	bus := eventbus.New()
	store := statestore.New(dir)

	goalID := "goal_chain_002"

	// Simulate a complete goal lifecycle
	evtChain := []events.Event{
		{Seq: 1, Type: events.TypeGoalCreated, GoalID: goalID, Source: "daemon"},
		{Seq: 2, Type: events.TypePlanRequested, GoalID: goalID, Source: "scheduler"},
		{Seq: 3, Type: events.TypeMissionGenerated, GoalID: goalID, Source: "mission-engine"},
		{Seq: 4, Type: events.TypeActionScheduled, GoalID: goalID, Source: "scheduler"},
		{Seq: 5, Type: events.TypeActionApproved, GoalID: goalID, Source: "governance"},
		{Seq: 6, Type: events.TypeActionCompleted, GoalID: goalID, Source: "plugin-runner"},
		{Seq: 7, Type: events.TypeGoalCompleted, GoalID: goalID, Source: "scheduler"},
	}

	received := make([]string, 0, len(evtChain))
	bus.Subscribe(events.TypeGoalCompleted, func(e events.Event) error {
		received = append(received, e.Type)
		return nil
	})

	for _, evt := range evtChain {
		store.Append(goalID, evt)
		bus.Publish(evt)
	}

	// Verify all 7 events persisted
	replayed, err := store.Replay(goalID, 0)
	if err != nil {
		t.Fatalf("Replay failed: %v", err)
	}
	if len(replayed) != 7 {
		t.Fatalf("expected 7 events, got %d", len(replayed))
	}

	// Verify GoalCompleted was delivered
	if len(received) != 1 {
		t.Errorf("expected 1 GoalCompleted delivered, got %d", len(received))
	}
}
