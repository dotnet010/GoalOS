package governance_test

import (
	"testing"

	"github.com/goalos/goalos/internal/eventbus"
	"github.com/goalos/goalos/internal/governance"
	"github.com/goalos/goalos/pkg/events"
)

func TestGovernance_LowRiskActionApproved(t *testing.T) {
	bus := eventbus.New()
	eng := governance.New(bus, nil)
	eng.RegisterCapabilities("test-plugin", []string{"fs.read", "fs.write", "browser.open", "browser.click", "shell.execute", "fs.delete"})
	eng.Start()

	approved := make(chan events.Event, 1)
	bus.Subscribe(events.TypeActionApproved, func(evt events.Event) error {
		approved <- evt
		return nil
	})

	bus.Publish(events.Event{
		Type:   events.TypeActionScheduled,
		GoalID: "goal_001",
		Source: "scheduler",
		Payload: map[string]interface{}{
			"action_id":             "act_001",
			"action_type":           "fs.read",
			"required_capabilities": []interface{}{"fs.read"},
		},
	})

	select {
	case evt := <-approved:
		actionID, _ := evt.Payload["action_id"].(string)
		if actionID != "act_001" {
			t.Errorf("expected act_001, got %s", actionID)
		}
	default:
		t.Fatal("ActionApproved was not published for L0 action")
	}
}

func TestGovernance_HighRiskActionPendingApproval(t *testing.T) {
	bus := eventbus.New()
	eng := governance.New(bus, nil)
	eng.RegisterCapabilities("test-plugin", []string{"shell.execute"})
	eng.Start()

	pending := make(chan events.Event, 1)
	bus.Subscribe(events.TypeActionPendingApproval, func(evt events.Event) error {
		pending <- evt
		return nil
	})

	bus.Publish(events.Event{
		Type:   events.TypeActionScheduled,
		GoalID: "goal_001",
		Source: "scheduler",
		Payload: map[string]interface{}{
			"action_id":   "act_002",
			"action_type": "shell.execute",
		},
	})

	select {
	case evt := <-pending:
		riskLevel, _ := evt.Payload["risk_level"].(string)
		if riskLevel < "L3" {
			t.Errorf("shell.execute should be L3+, got %s", riskLevel)
		}
	default:
		t.Fatal("ActionPendingApproval was not published for L3+ action")
	}
}

func TestGovernance_RiskScoring(t *testing.T) {
	tests := []struct {
		actionType   string
		expectL3Plus bool
	}{
		{"fs.read", false},
		{"fs.write", false},
		{"browser.click", false},
		{"shell.execute", true},
		{"fs.delete", true},
		{"database.delete", true},
		{"payment.initiate", true},
	}

	for _, tt := range tests {
		bus := eventbus.New()
		eng := governance.New(bus, nil)
		eng.RegisterCapabilities("test-plugin", []string{"fs.read", "fs.write", "browser.open", "browser.click", "shell.execute", "fs.delete", "database.delete", "payment.initiate"})
		eng.Start()

		pending := make(chan events.Event, 1)
		bus.Subscribe(events.TypeActionPendingApproval, func(evt events.Event) error {
			select {
			case pending <- evt:
			default:
			}
			return nil
		})

		bus.Publish(events.Event{
			Type:   events.TypeActionScheduled,
			GoalID: "goal_risk",
			Source: "scheduler",
			Payload: map[string]interface{}{
				"action_id":   "act_risk",
				"action_type": tt.actionType,
			},
		})

		select {
		case <-pending:
			if !tt.expectL3Plus {
				t.Errorf("%s: unexpected pending approval (should be L0-L2)", tt.actionType)
			}
		default:
			if tt.expectL3Plus {
				t.Errorf("%s: expected pending approval (L3+)", tt.actionType)
			}
		}
	}
}

// TestGovernance_ActionApprovedPayloadCompleteness 验证 ActionApproved payload
// 包含 PluginRunner 执行所需的全部字段（action_type/target/params/required_capabilities/timeout_seconds）。
// 这个测试直接防止 publishApproved 丢弃字段的回归 bug。
func TestGovernance_ActionApprovedPayloadCompleteness(t *testing.T) {
	bus := eventbus.New()
	eng := governance.New(bus, nil)
	eng.RegisterCapabilities("test-plugin", []string{"fs.write"})
	eng.Start()

	approved := make(chan events.Event, 1)
	bus.Subscribe(events.TypeActionApproved, func(evt events.Event) error {
		approved <- evt
		return nil
	})

	bus.Publish(events.Event{
		Type:   events.TypeActionScheduled,
		GoalID: "goal_payload",
		Source: "scheduler",
		Payload: map[string]interface{}{
			"action_id":             "act_payload_001",
			"action_type":           "fs.write",
			"target":                "/tmp/test.txt",
			"params":                map[string]interface{}{"encoding": "utf-8"},
			"required_capabilities": []interface{}{"fs.write"},
			"timeout_seconds":       float64(45),
			"risk_level_pre":        "L1",
		},
	})

	select {
	case evt := <-approved:
		// 验证核心字段转发
		if v, _ := evt.Payload["action_type"].(string); v != "fs.write" {
			t.Errorf("action_type: expected 'fs.write', got '%s'", v)
		}
		if v, _ := evt.Payload["target"].(string); v != "/tmp/test.txt" {
			t.Errorf("target: expected '/tmp/test.txt', got '%s'", v)
		}
		if _, ok := evt.Payload["params"]; !ok {
			t.Error("params: field missing from ActionApproved payload")
		}
		if _, ok := evt.Payload["required_capabilities"]; !ok {
			t.Error("required_capabilities: field missing from ActionApproved payload")
		}
		if v, _ := evt.Payload["timeout_seconds"].(float64); v != 45 {
			t.Errorf("timeout_seconds: expected 45, got %v", v)
		}
		// 验证 decision_path 字段存在
		if dp, ok := evt.Payload["decision_path"].(map[string]string); !ok {
			t.Error("decision_path: field missing from ActionApproved payload")
		} else if dp["risk"] == "" {
			t.Error("decision_path.risk: empty")
		}
	default:
		t.Fatal("ActionApproved was not published within 1s")
	}
}
