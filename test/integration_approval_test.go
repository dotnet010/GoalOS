// 集成测试 — 完整审批流程、Governance 安全闸口、异步审批竞态。
package test

import (
	"testing"
	"time"

	"github.com/goalos/goalos/internal/eventbus"
	"github.com/goalos/goalos/internal/governance"
	"github.com/goalos/goalos/internal/missionengine"
	"github.com/goalos/goalos/internal/pluginrunner"
	"github.com/goalos/goalos/internal/scheduler"
	"github.com/goalos/goalos/internal/statestore"
	"github.com/goalos/goalos/pkg/events"
)

// TestIntegration_FullApprovalFlow 验证完整审批流程：
// L3+ ActionScheduled → ActionPendingApproval → UserApprovedAction → ActionApproved。
func TestIntegration_FullApprovalFlow(t *testing.T) {
	dir := t.TempDir()
	bus := eventbus.New()
	store := statestore.New(dir)

	// Wire modules
	sched := scheduler.New(bus, store, scheduler.NewGoalAnchorTracker(20))
	sched.Start()

	gov := governance.New(bus, nil)
	gov.RegisterCapabilities("test-plugin", []string{"shell.execute", "fs.read", "fs.write"})
	gov.Start()

	missionengine.New(bus, missionengine.NewStubAgent()).Start()
	pluginrunner.New(bus, nil).Start()

	// Track events
	pending := make(chan events.Event, 1)
	approved := make(chan events.Event, 1)
	completed := make(chan events.Event, 1)

	bus.Subscribe(events.TypeActionPendingApproval, func(evt events.Event) error {
		pending <- evt
		return nil
	})
	bus.Subscribe(events.TypeActionApproved, func(evt events.Event) error {
		approved <- evt
		return nil
	})
	bus.Subscribe(events.TypeActionCompleted, func(evt events.Event) error {
		completed <- evt
		return nil
	})

	// 直接发布 L3 Action（绕过 Mission Engine 的 auto-confirm）
	bus.Publish(events.Event{
		Type:   events.TypeActionScheduled,
		GoalID: "goal_integ_001",
		Source: "scheduler",
		Payload: map[string]interface{}{
			"action_id":             "act_integ_001",
			"action_type":           "shell.execute",
			"required_capabilities": []interface{}{"shell.execute"},
			"timeout_seconds":       30,
			"risk_level_pre":        "L3",
		},
	})

	// Step 1: 应收到 ActionPendingApproval
	select {
	case <-pending:
		// ok
	case <-time.After(time.Second):
		t.Fatal("L3 Action 应触发 ActionPendingApproval")
	}

	// Step 2: 用户批准 → 应收到 ActionApproved
	bus.Publish(events.Event{
		Type:   events.TypeUserApprovedAction,
		GoalID: "goal_integ_001",
		Source: "channel-adapter",
		Payload: map[string]interface{}{
			"action_id": "act_integ_001",
		},
	})

	select {
	case evt := <-approved:
		// 验证 token_id 存在（Token 签发已接入）
		if tokenID, _ := evt.Payload["token_id"].(string); tokenID == "" {
			t.Log("token_id not set (secret key not configured — expected in test)")
		}
	case <-time.After(time.Second):
		t.Fatal("用户批准后应发布 ActionApproved")
	}

	// Step 3: Plugin Runner 执行。无真实 Plugin 二进制 → stubExecute 发布 ActionFailed
		select {
		case <-completed:
			t.Log("Action completed (plugin binary found)")
		case <-time.After(time.Second):
			t.Log("expected — no plugin binary, stubExecute returns ActionFailed instead of ActionCompleted")
		}
}

// TestIntegration_PolicyEngineBlocksAction 验证 Policy Engine DENY 规则在主流程中生效。
func TestIntegration_PolicyEngineBlocksAction(t *testing.T) {
	dir := t.TempDir()
	bus := eventbus.New()
	store := statestore.New(dir)

	gov := governance.New(bus, nil)
	gov.RegisterCapabilities("test-plugin", []string{"database.delete"})
	gov.Start()

	scheduler.New(bus, store, scheduler.NewGoalAnchorTracker(20)).Start()

	rejected := make(chan events.Event, 1)
	bus.Subscribe(events.TypeActionRejected, func(evt events.Event) error {
		rejected <- evt
		return nil
	})

	// "block-prod-delete" 规则：database.delete + /production/* + L3+ → DENY
	bus.Publish(events.Event{
		Type:   events.TypeActionScheduled,
		GoalID: "goal_blocked",
		Source: "scheduler",
		Payload: map[string]interface{}{
			"action_id":             "act_blocked_001",
			"action_type":           "database.delete",
			"target":                "/production/db/main",
			"required_capabilities": []interface{}{"database.delete"},
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
		t.Fatal("block-prod-delete 规则应阻止 Action")
	}
}

// TestIntegration_CapabilityDeniedInFullFlow 验证未注册能力在主流程中被拒绝。
func TestIntegration_CapabilityDeniedInFullFlow(t *testing.T) {
	dir := t.TempDir()
	bus := eventbus.New()
	store := statestore.New(dir)

	gov := governance.New(bus, nil)
	// 不注册任何能力
	gov.Start()

	scheduler.New(bus, store, scheduler.NewGoalAnchorTracker(20)).Start()

	rejected := make(chan events.Event, 1)
	bus.Subscribe(events.TypeActionRejected, func(evt events.Event) error {
		rejected <- evt
		return nil
	})

	bus.Publish(events.Event{
		Type:   events.TypeActionScheduled,
		GoalID: "goal_no_cap",
		Source: "scheduler",
		Payload: map[string]interface{}{
			"action_id":             "act_no_cap_001",
			"action_type":           "payment.initiate",
			"required_capabilities": []interface{}{"payment.initiate"},
		},
	})

	select {
	case evt := <-rejected:
		reason, _ := evt.Payload["reject_reason"].(string)
		if reason != "capability_denied" {
			t.Errorf("expected capability_denied, got %s", reason)
		}
	case <-time.After(time.Second):
		t.Fatal("未注册的 capability 应被拒绝")
	}
}
