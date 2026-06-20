// Governance 集成测试 — 验证错误路径（DENY/超时/异步审批）。
// 设计依据：R238、R239（决策模块最低 3+1 测试）。
package governance_test

import (
	"testing"
	"time"

	"github.com/goalos/goalos/internal/eventbus"
	"github.com/goalos/goalos/internal/governance"
	"github.com/goalos/goalos/pkg/events"
)

// TestGovernance_AsyncApprovalFlow 验证完整异步审批流程。
// 行为：L3+ Action→ActionPendingApproval→UserApprovedAction→ActionApproved。
func TestGovernance_AsyncApprovalFlow(t *testing.T) {
	bus := eventbus.New()
	gov := governance.New(bus, nil)
	gov.RegisterCapabilities("test-plugin", []string{"fs.read", "fs.write", "shell.execute"}); gov.Start()

	// Step 1: 发布 L3+ Action → 应收到 ActionPendingApproval
	pending := make(chan events.Event, 1)
	bus.Subscribe(events.TypeActionPendingApproval, func(evt events.Event) error {
		pending <- evt
		return nil
	})

	bus.Publish(events.Event{
		Type:   events.TypeActionScheduled,
		GoalID: "goal_async",
		Source: "scheduler",
		Payload: map[string]interface{}{
			"action_id":   "act_async_001",
			"action_type": "shell.execute", // L3 → 触发审批
		},
	})

	var pendingEvt events.Event
	select {
	case pendingEvt = <-pending:
		riskLevel, _ := pendingEvt.Payload["risk_level"].(string)
		if riskLevel < "L3" {
			t.Errorf("shell.execute 应 ≥ L3, got %s", riskLevel)
		}
	case <-time.After(time.Second):
		t.Fatal("ActionPendingApproval 未被发布")
	}

	// Step 2: 模拟用户批准 → 应收到 ActionApproved
	approved := make(chan events.Event, 1)
	bus.Subscribe(events.TypeActionApproved, func(evt events.Event) error {
		approved <- evt
		return nil
	})

	bus.Publish(events.Event{
		Type:   events.TypeUserApprovedAction,
		GoalID: "goal_async",
		Source: "channel-adapter",
		Payload: map[string]interface{}{
			"action_id": "act_async_001",
		},
	})

	select {
	case evt := <-approved:
		actionID, _ := evt.Payload["action_id"].(string)
		if actionID != "act_async_001" {
			t.Errorf("expected act_async_001, got %s", actionID)
		}
	case <-time.After(time.Second):
		t.Fatal("ActionApproved 未被发布（用户批准后）")
	}
}

// TestGovernance_L0AutoApprove 验证 L0 Action 自动批准（不触发审批）。
// 行为：L0 Action → ActionApproved 直接发布。不经过 ActionPendingApproval。
func TestGovernance_L0AutoApprove(t *testing.T) {
	bus := eventbus.New()
	gov := governance.New(bus, nil)
	gov.RegisterCapabilities("test-plugin", []string{"fs.read", "fs.write", "shell.execute"}); gov.Start()

	pendingReceived := false
	bus.Subscribe(events.TypeActionPendingApproval, func(evt events.Event) error {
		pendingReceived = true
		return nil
	})

	approved := make(chan events.Event, 1)
	bus.Subscribe(events.TypeActionApproved, func(evt events.Event) error {
		approved <- evt
		return nil
	})

	bus.Publish(events.Event{
		Type:   events.TypeActionScheduled,
		GoalID: "goal_l0",
		Source: "scheduler",
		Payload: map[string]interface{}{
			"action_id":   "act_l0_001",
			"action_type": "fs.read", // L0 → 自动批准
		},
	})

	select {
	case <-approved:
		if pendingReceived {
			t.Error("L0 Action 不应触发 PendingApproval")
		}
	case <-time.After(time.Second):
		t.Fatal("ActionApproved 未被发布（L0 应自动批准）")
	}
}
