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
