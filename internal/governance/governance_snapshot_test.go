// Governance 审批超时快照语义契约测试（R-1054, 会议 #191；R-1059 载荷键改名）。
// Q 需求层 1：超时配置仅对新发起的审批请求生效；
// 进行中的审批始终使用其创建时刻的快照值（计时器与事件载荷同源同值）。
// R-1059: 载荷键 timeout_seconds → approval_timeout_seconds（与执行超时解耦）。
package governance_test

import (
	"testing"
	"time"

	"github.com/goalos/goalos/internal/eventbus"
	"github.com/goalos/goalos/internal/governance"
	"github.com/goalos/goalos/pkg/events"
)

// TestGovernance_TimeoutSnapshot_InFlightUnaffected
// 进行中的审批在超时配置热更新后，仍使用创建时刻的快照值：
// ①载荷 timeout_seconds 等于创建时刻引擎值（非硬编码 300）；
// ②热更新为 10ms 后，进行中审批不得被新值秒杀。
func TestGovernance_TimeoutSnapshot_InFlightUnaffected(t *testing.T) {
	bus := eventbus.New()
	eng := governance.New(bus, nil)
	eng.RegisterCapabilities("test-plugin", []string{"shell.execute"})
	eng.Start()
	eng.SetApprovalTimeout(2 * time.Second)

	pending := make(chan events.Event, 1)
	bus.Subscribe(events.TypeActionPendingApproval, func(evt events.Event) error {
		pending <- evt
		return nil
	})

	bus.Publish(events.Event{
		Type:   events.TypeActionScheduled,
		GoalID: "goal_snapshot",
		Source: "scheduler",
		Payload: map[string]interface{}{
			"action_id":   "act_snap_001",
			"action_type": "shell.execute", // L3 → 需审批
		},
	})

	evt := <-pending
	got, _ := evt.Payload["approval_timeout_seconds"].(float64)
	if got != 2 {
		t.Fatalf("载荷 approval_timeout_seconds 应等于创建时刻引擎快照值 2s，got %v（禁止硬编码 300）", got)
	}

	// 热更新（等价 /api/system/reload 或 SIGHUP 后的 SetApprovalTimeout 接线）。
	eng.SetApprovalTimeout(10 * time.Millisecond)

	rejected := make(chan events.Event, 1)
	bus.Subscribe(events.TypeActionRejected, func(e events.Event) error {
		rejected <- e
		return nil
	})
	// 若 10ms 错误作用于进行中审批，此处必已收到 ActionRejected。
	select {
	case e := <-rejected:
		if e.Payload["action_id"] == "act_snap_001" {
			t.Fatal("进行中的审批不得被热更新后的超时值影响（快照语义违反）")
		}
	case <-time.After(200 * time.Millisecond):
		// 原审批仍按 2s 快照计时，200ms 内无超时拒绝。通过。
	}
}

// TestGovernance_TimeoutSnapshot_NewApprovalUsesNewValue
// 热更新后新发起的审批请求使用新的超时快照值。
func TestGovernance_TimeoutSnapshot_NewApprovalUsesNewValue(t *testing.T) {
	bus := eventbus.New()
	eng := governance.New(bus, nil)
	eng.RegisterCapabilities("test-plugin", []string{"shell.execute"})
	eng.Start()
	eng.SetApprovalTimeout(1 * time.Second)

	// 热更新：新超时 3s。仅对之后发起的审批生效。
	eng.SetApprovalTimeout(3 * time.Second)

	pending := make(chan events.Event, 1)
	bus.Subscribe(events.TypeActionPendingApproval, func(evt events.Event) error {
		pending <- evt
		return nil
	})

	bus.Publish(events.Event{
		Type:   events.TypeActionScheduled,
		GoalID: "goal_snapshot_new",
		Source: "scheduler",
		Payload: map[string]interface{}{
			"action_id":   "act_snap_002",
			"action_type": "shell.execute",
		},
	})

	evt := <-pending
	got, _ := evt.Payload["approval_timeout_seconds"].(float64)
	if got != 3 {
		t.Fatalf("新发起的审批应使用热更新后的快照值 3s，got %v", got)
	}
}

// TestGovernance_TimeoutSnapshot_PayloadMatchesEngineValue
// 载荷 timeout_seconds 必须等于引擎配置值——配置 600s 时载荷报 600，
// 杜绝"计时器按配置、载荷硬编码 300"的分裂。
func TestGovernance_TimeoutSnapshot_PayloadMatchesEngineValue(t *testing.T) {
	bus := eventbus.New()
	eng := governance.New(bus, nil)
	eng.RegisterCapabilities("test-plugin", []string{"shell.execute"})
	eng.Start()
	eng.SetApprovalTimeout(600 * time.Second)

	pending := make(chan events.Event, 1)
	bus.Subscribe(events.TypeActionPendingApproval, func(evt events.Event) error {
		pending <- evt
		return nil
	})

	bus.Publish(events.Event{
		Type:   events.TypeActionScheduled,
		GoalID: "goal_snapshot_cfg",
		Source: "scheduler",
		Payload: map[string]interface{}{
			"action_id":   "act_snap_003",
			"action_type": "shell.execute",
		},
	})

	evt := <-pending
	got, _ := evt.Payload["approval_timeout_seconds"].(float64)
	if got != 600 {
		t.Fatalf("载荷 approval_timeout_seconds 必须等于引擎配置值 600s，got %v", got)
	}
}
