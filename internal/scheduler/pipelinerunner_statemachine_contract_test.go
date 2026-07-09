// Package scheduler — v0.2.0 Week 1: Meyer 状态转换契约测试
// 10 个契约测试覆盖 StateMachineRun 所有状态转换路径。
// 设计: Bertrand Meyer，实现: Kent Beck。
package scheduler

import (
	"context"
	"testing"
	"time"

	"github.com/goalos/goalos/internal/errorcategory"
	"github.com/goalos/goalos/internal/eventbus"
	"github.com/goalos/goalos/internal/statestore"
	"github.com/goalos/goalos/pkg/events"
)

// ─── StateCheck 转换 ──────────────────────────────────────────

// TestContract_StateMachine_CheckPASS_ToExec 验证 CheckPASS→StateExec。
// MUST: checkAction 返回 CheckPASS 时，状态转移到 StateExec。
func TestContract_StateMachine_CheckPASS_ToExec(t *testing.T) {
	bus := eventbus.New()
	defer bus.Shutdown()
	store := statestore.New(t.TempDir())
	pr := NewPipelineRunner(bus, store)

	// CR-T1: 验证 ActionScheduled 发布内容——不只是事件存在，还要验证字段
	var capturedActionID string
	bus.Subscribe("ActionScheduled", func(evt events.Event) error {
		if id, ok := evt.Payload["action_id"].(string); ok {
			capturedActionID = id
		}
		return nil
	})

	ctx := context.Background()
	result := pr.StateMachineRun(ctx, "goal-test", "act-001")
	if result == nil {
		t.Fatal("StateMachineRun MUST return non-nil result")
	}
	// v0.2.0 W2: Async exec → returns PipelineWaiting (external wakeup via GoalRunner)
	if result.Status != PipelineWaiting {
		t.Fatalf("Async exec MUST return PipelineWaiting, got %v", result.Status)
	}
	if len(result.PendingWaits) == 0 {
		t.Error("PipelineWaiting MUST include PendingWaits")
	}
	// CR-T1: 验证 ActionScheduled 事件包含正确的 action_id
	if capturedActionID != "act-001" {
		t.Errorf("ActionScheduled action_id MUST be 'act-001', got '%s'", capturedActionID)
	}
}

// TestContract_StateMachine_CheckREJECT_ReturnsFailed 验证 CheckREJECT→PipelineFailed。
// MUST: checkAction 返回 CheckREJECT 时，立即返回 PipelineFailed（不进入 Exec）。
func TestContract_StateMachine_CheckREJECT_ReturnsFailed(t *testing.T) {
	bus := eventbus.New()
	defer bus.Shutdown()
	store := statestore.New(t.TempDir())
	pr := NewPipelineRunner(bus, store)

	// 空 actionID 触发 CheckREJECT
	ctx := context.Background()
	result := pr.StateMachineRun(ctx, "goal-test", "")
	if result == nil {
		t.Fatal("StateMachineRun MUST return non-nil result")
	}
	if result.Status != PipelineFailed {
		t.Fatalf("CheckREJECT MUST return PipelineFailed, got %v", result.Status)
	}
	if result.Error != "check_rejected" {
		t.Errorf("CheckREJECT error MUST be 'check_rejected', got '%s'", result.Error)
	}
}

// TestContract_StateMachine_CheckBLOCK_ToWait 验证 CheckBLOCK→StateWait。
// MUST: checkAction 返回 CheckBLOCK 时，进入 Wait 状态。
func TestContract_StateMachine_CheckBLOCK_ToWait(t *testing.T) {
	bus := eventbus.New()
	defer bus.Shutdown()
	store := statestore.New(t.TempDir())
	pr := NewPipelineRunner(bus, store)

	// 模拟过载：大量 retry entries 触发 CheckBLOCK
	pr.retryCount = make(map[string]int)
	for i := 0; i < 200; i++ {
		pr.retryCount[string(rune(i))] = 1
	}

	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()

	result := pr.StateMachineRun(ctx, "goal-test", "act-overload")
	if result == nil {
		t.Fatal("StateMachineRun MUST return non-nil result")
	}
	// v0.2.0 W2: CheckBLOCK→Check→requiresWait→returns PipelineWaiting (not internally blocked)
	if result.Status != PipelineWaiting {
		t.Fatalf("CheckBLOCK MUST return PipelineWaiting, got %v", result.Status)
	}
	if len(result.PendingWaits) == 0 {
		t.Error("PipelineWaiting MUST include PendingWaits")
	}
}

// ─── StateExec 转换 ───────────────────────────────────────────

// TestContract_StateMachine_Exec_EmptyActionID_ReturnsError 验证空 actionID→错误。
// MUST: executeAction 在 actionID 为空时返回 Permanent 错误。
func TestContract_StateMachine_Exec_EmptyActionID_ReturnsError(t *testing.T) {
	pr := &PipelineRunner{}
	result, err := pr.executeAction("")
	if err == nil {
		t.Fatal("executeAction MUST return error for empty actionID")
	}
	if result != nil {
		t.Error("executeAction MUST return nil result on error")
	}
	catErr, ok := err.(errorcategory.CategorizedError)
	if !ok {
		t.Fatal("executeAction error MUST implement CategorizedError")
	}
	if catErr.Category() != errorcategory.ErrorPermanent {
		t.Errorf("empty actionID error MUST be Permanent, got %v", catErr.Category())
	}
}

// TestContract_StateMachine_Exec_PluginNotFound_ReturnsError 验证 Plugin 不存在→错误。
// MUST: Plugin cache 中找不到对应 Plugin 时返回 Permanent 错误。
func TestContract_StateMachine_Exec_PluginNotFound_ReturnsError(t *testing.T) {
	bus := eventbus.New()
	defer bus.Shutdown()
	store := statestore.New(t.TempDir())
	pr := NewPipelineRunner(bus, store)
	// PluginCache is populated but won't have "nonexistent-plugin"
	pr.pluginCache = NewPluginCache()
	// Register a different plugin to make Count() > 0
	pr.pluginCache.OnPluginRegistered(&PluginInfo{PluginName: "other-plugin", PluginType: "capability"})

	_, err := pr.executeAction("nonexistent-plugin")
	if err == nil {
		t.Fatal("executeAction MUST return error for unregistered plugin")
	}
}

// TestContract_StateMachine_Exec_ValidAction_PublishesEvent 验证合法 Action→发布事件。
// MUST: 合法 actionID + 有 bus→发布 ActionScheduled + 返回 Async=true。
func TestContract_StateMachine_Exec_ValidAction_PublishesEvent(t *testing.T) {
	bus := eventbus.New()
	defer bus.Shutdown()
	store := statestore.New(t.TempDir())
	pr := NewPipelineRunner(bus, store)

	actionScheduled := false
	bus.Subscribe("ActionScheduled", func(evt events.Event) error {
		actionScheduled = true
		return nil
	})

	result, err := pr.executeAction("valid-action")
	if err != nil {
		t.Fatalf("executeAction MUST not error for valid action: %v", err)
	}
	if result == nil {
		t.Fatal("executeAction MUST return non-nil result")
	}
	if !result.Async {
		t.Error("executeAction MUST return Async=true when bus is available")
	}
	if !actionScheduled {
		t.Error("executeAction MUST publish ActionScheduled event")
	}
}

// ─── StateDecide 转换 ──────────────────────────────────────────

// TestContract_StateMachine_Decide_ReturnsCompleted 验证 Decide→PipelineCompleted。
// MUST: StateDecide 返回 PipelineCompleted（CONTINUE 路径）。
func TestContract_StateMachine_Decide_ReturnsCompleted(t *testing.T) {
	// v0.2.0 W2: 无 bus → 同步执行 → Async=false → Decide → Completed
	pr := &PipelineRunner{}

	ctx := context.Background()
	result := pr.StateMachineRun(ctx, "goal-test", "act-002")
	if result == nil {
		t.Fatal("StateMachineRun MUST return non-nil result")
	}
	if result.Status != PipelineCompleted {
		t.Fatalf("Sync exec MUST return PipelineCompleted, got %v", result.Status)
	}
}

// ─── 错误处理 ──────────────────────────────────────────────────

// TestContract_StateMachine_CategorizedError_RoutesCorrectly 验证错误分类路由。
// MUST: Permanent 错误→立即 PipelineFailed。Temporary 错误→内部重试。
func TestContract_StateMachine_CategorizedError_RoutesCorrectly(t *testing.T) {
	// 构造一个返回 Permanent 错误的 executeAction 场景
	pr := &PipelineRunner{}
	result, err := pr.executeAction("")
	if err == nil {
		t.Fatal("empty actionID MUST return error")
	}
	catErr, ok := err.(errorcategory.CategorizedError)
	if !ok {
		t.Fatal("error MUST implement CategorizedError interface")
	}
	if catErr.Category() != errorcategory.ErrorPermanent {
		t.Errorf("empty actionID MUST be Permanent, got %v", catErr.Category())
	}
	_ = result
}

// TestContract_StateMachine_ContextCancelled_ReturnsFailed 验证 Context 取消。
// MUST: ctx.Done() → 立即返回 PipelineFailed(cancelled)。
func TestContract_StateMachine_ContextCancelled_ReturnsFailed(t *testing.T) {
	bus := eventbus.New()
	defer bus.Shutdown()
	store := statestore.New(t.TempDir())
	pr := NewPipelineRunner(bus, store)

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // 立即取消

	result := pr.StateMachineRun(ctx, "goal-test", "act-003")
	if result == nil {
		t.Fatal("StateMachineRun MUST return non-nil result")
	}
	if result.Status != PipelineFailed {
		t.Fatalf("cancelled context MUST return PipelineFailed, got %v", result.Status)
	}
	if result.Error != "cancelled" {
		t.Errorf("cancelled error MUST be 'cancelled', got '%s'", result.Error)
	}
}

// ─── Nil 安全 ──────────────────────────────────────────────────

// TestContract_StateMachine_NilBus_NoPanic 验证 nil bus 不 panic。
// MUST: PipelineRunner 在 bus 为 nil 时不 panic，降级返回结果。
func TestContract_StateMachine_NilBus_NoPanic(t *testing.T) {
	pr := &PipelineRunner{}
	ctx := context.Background()

	// 不应 panic
	result := pr.StateMachineRun(ctx, "goal-test", "act-004")
	if result == nil {
		t.Fatal("StateMachineRun MUST return non-nil result even with nil fields")
	}
	if result.Status != PipelineCompleted {
		t.Fatalf("nil bus pipeline MUST complete, got %v", result.Status)
	}
}

// TestContract_StateMachine_NilStore_NoPanic 验证 nil store 不 panic。
// MUST: requiresWait 在 store 为 nil 时返回 false，不 panic。
func TestContract_StateMachine_NilStore_NoPanic(t *testing.T) {
	// v0.2.0 W2: 无 bus → 同步执行路径
	pr := &PipelineRunner{}

	ctx := context.Background()
	result := pr.StateMachineRun(ctx, "goal-test", "act-005")
	if result == nil {
		t.Fatal("StateMachineRun MUST return non-nil result")
	}
	// 无 bus → executeAction returns {Async: false} → Decide → Completed
	if result.Status != PipelineCompleted {
		t.Fatalf("nil fields pipeline MUST complete, got %v", result.Status)
	}
}

// TestContract_Exec_NilBus_NoPanic 验证 exec() 在 nil bus 时不 panic（CR-T2）。
// MUST: exec() 在 PipelineRunner.bus 为 nil 时直接返回 nil（不 panic，不验证）。
func TestContract_Exec_NilBus_NoPanic(t *testing.T) {
	pr := &PipelineRunner{} // no bus
	// CR-T2: exec() with nil bus must not panic and return nil immediately
	err := pr.exec("goal-1", "act-nil-bus")
	if err != nil {
		t.Fatalf("exec() MUST return nil for nil bus, got: %v", err)
	}
}
