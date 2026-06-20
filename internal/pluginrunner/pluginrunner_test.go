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

	// 无真实 Plugin 二进制 → stubExecute 发布 ActionFailed
	done := make(chan events.Event, 1)
	bus.Subscribe(events.TypeActionFailed, func(evt events.Event) error {
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
		if result["status"] != "failure" {
			t.Errorf("expected failure (no plugin binary), got %s", result["status"])
		}
	case <-time.After(time.Second):
		t.Fatal("ActionFailed was not published within 1s")
	}
}

func TestPluginRunner_MultipleActions(t *testing.T) {
	bus := eventbus.New()
	runner := pluginrunner.New(bus)
	runner.Start()

	count := 0
	done := make(chan struct{})
	// 无真实 Plugin → stubExecute 发布 ActionFailed（非 ActionCompleted）
	bus.Subscribe(events.TypeActionFailed, func(evt events.Event) error {
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
			t.Errorf("expected 5 failures (no plugin binaries), got %d", count)
		}
	case <-time.After(time.Second):
		t.Fatalf("only %d/5 failures received", count)
	}
}
