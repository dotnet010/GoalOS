// Package test — v0.2.0 Week 2 contract_test
// Beck 编写（R-571 测试先行）。
package test

import (
	"context"
	"testing"
	"time"

	"github.com/goalos/goalos/internal/scheduler"
)

// ─── A8: PipelineRunner.StateMachineRun() ──────────────────────

func TestA8_PipelineRunner_StateMachine(t *testing.T) {
	t.Run("Run返回非nil结果", func(t *testing.T) {
		pr := &scheduler.PipelineRunner{}
		ctx := context.Background()
		result := pr.StateMachineRun(ctx, "goal-test", "action-1")
		if result == nil {
			t.Fatal("StateMachineRun 不应返回 nil——A8 空壳检测")
		}
		if result.Status != scheduler.PipelineCompleted {
			t.Errorf("Status=%v, want PipelineCompleted", result.Status)
		}
	})

	t.Run("空actionID_Check返回REJECT_Run返回Failed", func(t *testing.T) {
		pr := &scheduler.PipelineRunner{}
		ctx := context.Background()
		result := pr.StateMachineRun(ctx, "goal-test", "")
		if result == nil {
			t.Fatal("空 actionID 应返回非 nil")
		}
		if result.Status != scheduler.PipelineFailed {
			t.Errorf("空 actionID: Status=%v, want PipelineFailed（Check 应返回 REJECT）", result.Status)
		}
	})

	t.Run("ctx取消_返回PipelineFailed", func(t *testing.T) {
		pr := &scheduler.PipelineRunner{}
		ctx, cancel := context.WithCancel(context.Background())
		cancel()
		result := pr.StateMachineRun(ctx, "goal-test", "action-1")
		if result.Status != scheduler.PipelineFailed {
			t.Errorf("取消的 ctx 应返回 PipelineFailed, got %v", result.Status)
		}
	})

	t.Run("合法actionID_状态机正常完成", func(t *testing.T) {
		pr := &scheduler.PipelineRunner{}
		ctx := context.Background()
		result := pr.StateMachineRun(ctx, "goal-test", "valid-action")
		if result == nil {
			t.Fatal("合法 actionID 应返回非 nil 结果")
		}
		// CheckPASS→Exec→Decide→PipelineCompleted
		if result.Status != scheduler.PipelineCompleted {
			t.Errorf("Status=%v, want PipelineCompleted", result.Status)
		}
	})
}

// ─── I2: GoalRunner 两阶段 select ──────────────────────────────

func TestI2_GoalRunner_Select(t *testing.T) {
	t.Run("Pause指令改变状态为Paused", func(t *testing.T) {
		gs := scheduler.NewGoalState("goal-1")
		gs.SetState("Running")
		stopCh := make(chan struct{})
		go gs.Run(stopCh)
		gs.ControlChan <- scheduler.ControlPause
		time.Sleep(50 * time.Millisecond)
		close(stopCh)
		if state := gs.GetState(); state != "Paused" {
			t.Errorf("Pause 后 State=%v, want Paused", state)
		}
	})

	t.Run("Stop指令改变状态为Failed", func(t *testing.T) {
		gs := scheduler.NewGoalState("goal-2")
		gs.SetState("Running")
		stopCh := make(chan struct{})
		go gs.Run(stopCh)
		gs.ControlChan <- scheduler.ControlStop
		time.Sleep(50 * time.Millisecond)
		close(stopCh)
		if state := gs.GetState(); state != "Failed" {
			t.Errorf("Stop 后 State=%v, want Failed", state)
		}
	})

	t.Run("Paused状态不接受唤醒", func(t *testing.T) {
		gs := scheduler.NewGoalState("goal-3")
		gs.SetState("Paused")
		stopCh := make(chan struct{})
		go gs.Run(stopCh)
		gs.WakeupChan <- scheduler.WakeupEvent{Type: "approval", ActionID: "a1"}
		time.Sleep(50 * time.Millisecond)
		close(stopCh)
		if state := gs.GetState(); state != "Paused" {
			t.Errorf("Paused 状态应保持 Paused, got %v", state)
		}
	})
}

// ─── I3: Goal 终态判定 ─────────────────────────────────────────

func TestI3_Goal_TerminalState(t *testing.T) {
	t.Run("Running不是终态", func(t *testing.T) {
		gs := scheduler.NewGoalState("goal-1")
		gs.SetState("Running")
		if gs.IsTerminal() {
			t.Error("Running 不应是终态")
		}
	})

	t.Run("Completed是终态", func(t *testing.T) {
		gs := scheduler.NewGoalState("goal-2")
		gs.SetState("Completed")
		if !gs.IsTerminal() {
			t.Error("Completed 应是终态")
		}
	})

	t.Run("Failed是终态", func(t *testing.T) {
		gs := scheduler.NewGoalState("goal-3")
		gs.SetState("Failed")
		if !gs.IsTerminal() {
			t.Error("Failed 应是终态")
		}
	})
}

// ─── J3: TokenBudget Graceful Wait ──────────────────────────────

func TestJ3_TokenBudget_GracefulWait(t *testing.T) {
	t.Run("BudgetTracker存在并可追踪token消耗", func(t *testing.T) {
		// v0.3.0 fix (H6): 验证 BudgetTracker 正确初始化和基本操作
		tracker := scheduler.NewBudgetTrackerWithBudget(10000)
		if tracker == nil {
			t.Fatal("BudgetTracker MUST NOT be nil")
		}
		if used := tracker.Used(); used != 0 {
			t.Errorf("initial Used MUST be 0, got %d", used)
		}
		// 记录 500 token 消耗——不应超出预算
		tracker.RecordUsageSimple(500)
		if tracker.IsExceededSimple(10000) {
			t.Error("RecordUsageSimple(500) within 10000 budget MUST NOT be exceeded")
		}
		if used := tracker.Used(); used != 500 {
			t.Errorf("Used after RecordUsageSimple(500) MUST be 500, got %d", used)
		}
		// 超出预算
		tracker.RecordUsageSimple(10000)
		if !tracker.IsExceededSimple(10000) {
			t.Error("IsExceededSimple(10000) MUST return true after RecordUsageSimple(10000)")
		}
	})
}

// ─── Plugin Fan-Out 本地 cache ─────────────────────────────────

func TestPluginFanOut_LocalCache(t *testing.T) {
	cache := scheduler.NewPluginCache()

	t.Run("Register后Lookup返回Plugin", func(t *testing.T) {
		cache.OnPluginRegistered(&scheduler.PluginInfo{
			PluginName:   "web-search",
			PluginType:   "capability",
			Capabilities: []string{"web.search"},
		})
		p, ok := cache.Lookup("web-search")
		if !ok {
			t.Fatal("Register 后 Lookup 应返回 Plugin")
		}
		if p.PluginName != "web-search" {
			t.Errorf("PluginName=%v, want web-search", p.PluginName)
		}
	})

	t.Run("Unregister后Lookup返回nil", func(t *testing.T) {
		cache.OnPluginRegistered(&scheduler.PluginInfo{PluginName: "test-plugin"})
		cache.OnPluginUnregistered("test-plugin")
		_, ok := cache.Lookup("test-plugin")
		if ok {
			t.Error("Unregister 后 Lookup 应返回 false")
		}
	})

	t.Run("空cache_Count为0", func(t *testing.T) {
		empty := scheduler.NewPluginCache()
		if empty.Count() != 0 {
			t.Errorf("空 cache Count=%d, want 0", empty.Count())
		}
	})

	t.Run("多个Register_Count正确", func(t *testing.T) {
		multi := scheduler.NewPluginCache()
		multi.OnPluginRegistered(&scheduler.PluginInfo{PluginName: "a"})
		multi.OnPluginRegistered(&scheduler.PluginInfo{PluginName: "b"})
		multi.OnPluginRegistered(&scheduler.PluginInfo{PluginName: "c"})
		if multi.Count() != 3 {
			t.Errorf("Count=%d, want 3", multi.Count())
		}
	})
}
