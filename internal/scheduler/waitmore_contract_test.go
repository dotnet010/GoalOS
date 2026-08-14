// 契约测试：wait_more 必须重写 PipelineState.TimeoutAt（R-1376）。
//
// 断言来源: R-1376（"wait_more 双重写=延长 governance 计时器且重写
//   PipelineState.TimeoutAt（同一 approval_timeout 窗口）"）+ R-1379（wait_more
//   端点保留）+ 07 事件注册表 §3 UserDecisionReceived（R-1161/R-1211：
//   decision 枚举 "approve"|"reject"|"wait_more"，"wait_more 经事件投递 GoalRunner——
//   取消+重挂定时器，世代号防陈旧触发"）。
//
// 先红状态（2026-08-14）: 无 wait_more 决策路径——UserDecisionReceived{decision:
//   "wait_more"} 事件发布后无人消费（GoalRunner.waitForWakeup 订阅集不含它，
//   governance 亦不订阅），快照中 PipelineState.TimeoutAt 保持原值 → 断言"被重写为
//   晚于原值的时刻"失败 → 红。
//
// 转绿任务: 1.27/1.28（计划 C-2 表——R-1376：wait_more 经事件投递 GoalRunner，
//   取消+重挂计时器并重写 TimeoutAt、持久化到快照；同时注册
//   events.TypeUserDecisionReceived 常量）。
//
// 契约 MUST（R-1376）:
//   - MUST 1: wait 状态收到 wait_more 决策后，持久化的 PipelineState.TimeoutAt 必须
//     被重写为晚于原值的时刻（延长；同一 approval_timeout 窗口）。
//
// 纪律: 禁止源码文本断言——行为断言：发布 wait_more 决策事件 → 观察快照中
//   PipelineState.TimeoutAt 是否被延长。测试自清理（t.TempDir 隔离 store）。

package scheduler

import (
	"testing"
	"time"

	"github.com/goalos/goalos/internal/eventbus"
	"github.com/goalos/goalos/internal/statestore"
	"github.com/goalos/goalos/pkg/events"
)

// waitMoreDecisionEventType 是 wait_more 决策的投递事件 wire 值（07 事件注册表
// UserDecisionReceived——R-1161 D26① 注册："wait_more 经事件投递 GoalRunner"）。
// pkg/events 尚未导出对应常量——转绿任务应注册 events.TypeUserDecisionReceived；
// 本测试以 wire 值字面量断言行为。
const waitMoreDecisionEventType = "UserDecisionReceived"

func TestWaitMore_ExtendsGoalRunnerTimeout(t *testing.T) {
	goalID := "goal-waitmore"
	bus := eventbus.New()
	store := statestore.New(t.TempDir())
	pr := NewPipelineRunner(bus, store)
	pr.SetApprovalTimeout(300 * time.Millisecond) // 短窗口：观测在 ms 级完成

	// wait() 前置：Run() 会初始化 pr.state（R-1342 签名唯一）——本测试直接进入
	// wait 原语，按 Run 的初始化语义设置空 PipelineState（测试装配，非生产改动）。
	pr.state = &PipelineState{}

	// PipelineRunner 进入 wait——TimeoutAt = now + approvalTimeout（R-1384 单一计时权威）。
	result, err := pr.wait(goalID, string(WaitApproval))
	if err != nil {
		t.Fatalf("pr.wait 失败: %v", err)
	}
	original := result.PipelineState.TimeoutAt
	if original == "" {
		t.Fatal("pr.wait 未设置 PipelineState.TimeoutAt——前置条件不成立")
	}

	gr := NewGoalRunner(Goal{ID: goalID}, bus, store, pr, NewGoalAnchorTracker(3))
	// 复刻 Execute() 的 PipelineWaiting 分支：持久化 PipelineState 并进入 waitForWakeup。
	if err := gr.savePipelineState(result.PipelineState); err != nil {
		t.Fatalf("savePipelineState 失败: %v", err)
	}
	done := make(chan struct{})
	go func() {
		gr.waitForWakeup(result)
		close(done)
	}()

	// wait_more 决策路径（R-1376/R-1379）: UserDecisionReceived{decision:"wait_more"}。
	bus.Publish(events.Event{
		Type:   waitMoreDecisionEventType,
		GoalID: goalID,
		Source: "test",
		Payload: map[string]interface{}{
			"action_id": "act-1",
			"decision":  "wait_more",
		},
	})

	// 契约（R-1376 双写）: 快照中 TimeoutAt 必须被重写为晚于原值的时刻。
	// 轮询窗口 3s——红态下事件无人消费，TimeoutAt 永不变化，轮询耗尽后失败。
	deadline := time.Now().Add(3 * time.Second)
	extended := false
	rewritten := ""
	for time.Now().Before(deadline) {
		state, err := store.LoadLatestSnapshot(goalID)
		if err == nil && state != nil && state.PipelineState != nil && state.PipelineState.TimeoutAt != "" {
			rewritten = state.PipelineState.TimeoutAt
			orig, errO := time.Parse(time.RFC3339, original)
			newT, errN := time.Parse(time.RFC3339, rewritten)
			if errO == nil && errN == nil && newT.After(orig) {
				extended = true
				break
			}
		}
		time.Sleep(20 * time.Millisecond)
	}
	if !extended {
		t.Errorf("MUST 1 失败（R-1376 wait_more 双写）: wait_more 决策后 PipelineState.TimeoutAt 未被延长——原值=%s 当前快照值=%s；无 wait_more 决策路径消费 UserDecisionReceived 事件", original, rewritten)
	}
	<-done
}
