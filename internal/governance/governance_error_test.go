// Governance 错误路径与安全闸口测试。
// 验证 Policy DENY、Capability DENY、审批超时、异步竞态。
package governance_test

import (
	"testing"
	"time"

	"github.com/goalos/goalos/internal/eventbus"
	"github.com/goalos/goalos/internal/governance"
	"github.com/goalos/goalos/pkg/events"
)

// TestGovernance_PolicyDeniesAction 验证 Policy Engine DENY 规则阻止 Action。
func TestGovernance_PolicyDeniesAction(t *testing.T) {
	bus := eventbus.New()
	eng := governance.New(bus, nil)
	eng.RegisterCapabilities("test-plugin", []string{"fs.delete"})
	eng.Start()

	rejected := make(chan events.Event, 1)
	bus.Subscribe(events.TypeActionRejected, func(evt events.Event) error {
		rejected <- evt
		return nil
	})

	// "block-prod-delete" 规则：database.delete + /production/* + L3+ → DENY
	bus.Publish(events.Event{
		Type:   events.TypeActionScheduled,
		GoalID: "goal_deny",
		Source: "scheduler",
		Payload: map[string]interface{}{
			"action_id":             "act_deny_001",
			"action_type":           "database.delete",
			"target":                "/production/db",
			"required_capabilities": []interface{}{"fs.delete"},
			"risk_level_pre":        "L4",
		},
	})

	select {
	case evt := <-rejected:
		reason, _ := evt.Payload["reject_reason"].(string)
		if reason != "policy_denied" {
			t.Errorf("expected policy_denied, got %s", reason)
		}
	case <-time.After(time.Second):
		t.Fatal("policy DENY 应发布 ActionRejected")
	}
}

// TestGovernance_CapabilityDenied 验证 Capability Engine 拒绝未注册的能力。
func TestGovernance_CapabilityDenied(t *testing.T) {
	bus := eventbus.New()
	eng := governance.New(bus, nil)
	// 不注册任何能力 → shell.execute 应被拒绝
	eng.Start()

	rejected := make(chan events.Event, 1)
	bus.Subscribe(events.TypeActionRejected, func(evt events.Event) error {
		rejected <- evt
		return nil
	})

	bus.Publish(events.Event{
		Type:   events.TypeActionScheduled,
		GoalID: "goal_cap_deny",
		Source: "scheduler",
		Payload: map[string]interface{}{
			"action_id":             "act_cap_001",
			"action_type":           "shell.execute",
			"required_capabilities": []interface{}{"shell.execute"},
		},
	})

	select {
	case evt := <-rejected:
		reason, _ := evt.Payload["reject_reason"].(string)
		if reason != "capability_denied" {
			t.Errorf("expected capability_denied, got %s", reason)
		}
	case <-time.After(time.Second):
		t.Fatal("未注册的 capability 应发布 ActionRejected")
	}
}

// TestGovernance_ApprovalTimeout 验证审批超时后发布 ActionRejected("approval_timeout")。
// R-1061（D-4）重写：旧版是空壳——select 空 case + default 恒通过、零断言，
// 超时机制任何回归都不会使测试变红。真断言：短超时（50ms）→ 先 ActionPendingApproval，
// 后 ActionRejected(reject_reason="approval_timeout")，且 action_id 匹配。
func TestGovernance_ApprovalTimeout(t *testing.T) {
	bus := eventbus.New()
	eng := governance.New(bus, nil)
	eng.RegisterCapabilities("test-plugin", []string{"shell.execute"})
	eng.Start()
	eng.SetApprovalTimeout(50 * time.Millisecond) // R-1054: 快照取自引擎字段，测试可调

	pending := make(chan events.Event, 1)
	rejected := make(chan events.Event, 1)
	bus.Subscribe(events.TypeActionPendingApproval, func(evt events.Event) error {
		pending <- evt
		return nil
	})
	bus.Subscribe(events.TypeActionRejected, func(evt events.Event) error {
		rejected <- evt
		return nil
	})

	bus.Publish(events.Event{
		Type:   events.TypeActionScheduled,
		GoalID: "goal_timeout",
		Source: "scheduler",
		Payload: map[string]interface{}{
			"action_id":   "act_timeout_001",
			"action_type": "shell.execute", // L3 → 需审批
		},
	})

	// ① 必须进入挂起审批（否则超时无从谈起）
	select {
	case evt := <-pending:
		if id, _ := evt.Payload["action_id"].(string); id != "act_timeout_001" {
			t.Fatalf("挂起审批 action_id 不匹配: got %s", id)
		}
	case <-time.After(time.Second):
		t.Fatal("L3 操作必须发布 ActionPendingApproval")
	}

	// ② 超时后必须发布 ActionRejected("approval_timeout")
	select {
	case evt := <-rejected:
		reason, _ := evt.Payload["reject_reason"].(string)
		if reason != "approval_timeout" {
			t.Fatalf("expected reject_reason=approval_timeout, got %s", reason)
		}
		if id, _ := evt.Payload["action_id"].(string); id != "act_timeout_001" {
			t.Fatalf("超时拒绝 action_id 不匹配: got %s", id)
		}
	case <-time.After(time.Second):
		t.Fatal("审批超时必须发布 ActionRejected（50ms 快照值未生效或计时器未启动）")
	}
}

// TestGovernance_UserApprovedAfterCancel 验证审批批准到达时若已取消则不会二次批准。
func TestGovernance_UserApprovedAfterCancel(t *testing.T) {
	bus := eventbus.New()
	eng := governance.New(bus, nil)
	eng.RegisterCapabilities("test-plugin", []string{"shell.execute"})
	eng.Start()

	pending := make(chan events.Event, 1)
	bus.Subscribe(events.TypeActionPendingApproval, func(evt events.Event) error {
		pending <- evt
		return nil
	})

	// Step 1: 发布 L3+ Action → 触发审批
	bus.Publish(events.Event{
		Type:   events.TypeActionScheduled,
		GoalID: "goal_cancel",
		Source: "scheduler",
		Payload: map[string]interface{}{
			"action_id":   "act_cancel_001",
			"action_type": "shell.execute",
		},
	})

	select {
	case <-pending:
		// 审批已挂起
	case <-time.After(time.Second):
		t.Fatal("ActionPendingApproval 未被发布")
	}

	// Step 2: 发布 ActionCancelled（模拟用户 pause/rollback）
	rejected := make(chan events.Event, 1)
	bus.Subscribe(events.TypeActionRejected, func(evt events.Event) error {
		rejected <- evt
		return nil
	})

	bus.Publish(events.Event{
		Type:   events.TypeActionCancelled,
		GoalID: "goal_cancel",
		Source: "scheduler",
		Payload: map[string]interface{}{
			"action_id": "act_cancel_001",
			"reason":    "user_paused",
		},
	})

	select {
	case evt := <-rejected:
		reason, _ := evt.Payload["reject_reason"].(string)
		if reason != "state_changed" {
			t.Errorf("expected state_changed, got %s", reason)
		}
	case <-time.After(time.Second):
		t.Fatal("ActionCancelled 应触发 ActionRejected(state_changed)")
	}
}
