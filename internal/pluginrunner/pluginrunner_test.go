package pluginrunner_test

import (
	"testing"
	"time"

	"github.com/goalos/goalos/internal/eventbus"
	"github.com/goalos/goalos/internal/pluginrunner"
	"github.com/goalos/goalos/pkg/events"
)

func TestPluginRunner_ActionApproved(t *testing.T) {
	bus := eventbus.New()
	runner := pluginrunner.New(bus)
	runner.Start()

	done := make(chan events.Event, 1)
	bus.Subscribe(events.TypeActionCompleted, func(evt events.Event) error {
		done <- evt
		return nil
	})

	bus.Publish(events.Event{
		Type:   events.TypeActionApproved,
		GoalID: "goal_001",
		Source: "governance",
		Payload: map[string]interface{}{
			"action_id":   "act_001",
			"action_type": "fs.read",
		},
	})

	select {
	case evt := <-done:
		actionID, _ := evt.Payload["action_id"].(string)
		if actionID != "act_001" {
			t.Errorf("expected act_001, got %s", actionID)
		}
		result, _ := evt.Payload["result"].(map[string]interface{})
		if result["status"] != "success" {
			t.Errorf("expected success, got %s", result["status"])
		}
	case <-time.After(time.Second):
		t.Fatal("ActionCompleted was not published within 1s")
	}
}

func TestPluginRunner_MultipleActions(t *testing.T) {
	bus := eventbus.New()
	runner := pluginrunner.New(bus)
	runner.Start()

	count := 0
	done := make(chan struct{})
	bus.Subscribe(events.TypeActionCompleted, func(evt events.Event) error {
		count++
		if count >= 5 {
			close(done)
		}
		return nil
	})

	for i := 1; i <= 5; i++ {
		bus.Publish(events.Event{
			Type:   events.TypeActionApproved,
			GoalID: "goal_multi",
			Source: "governance",
			Payload: map[string]interface{}{
				"action_id":   "act_multi_" + string(rune('0'+i)),
				"action_type": "fs.read",
			},
		})
	}

	select {
	case <-done:
		if count != 5 {
			t.Errorf("expected 5 completions, got %d", count)
		}
	case <-time.After(time.Second):
		t.Fatalf("only %d/5 completions received", count)
	}
}
