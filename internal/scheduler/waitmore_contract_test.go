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
	// 审批窗口 5s：必须大于本机调度延迟极值（观测到 >1s 的 goroutine 启动+6 路订阅
	// 注册延迟——窗口过短时 waitForWakeup 在订阅建立前已超时，wait_more 事件丢失）。
	pr.SetApprovalTimeout(5 * time.Second)

	// wait() 前置：Run() 会初始化 pr.state（R-1342 签名唯一）——本测试直接进入
	// wait 原语，按 Run 的初始化语义设置空 PipelineState（测试装配，非生产改动）。
	pr.state = &statestore.PipelineSnapshot{}

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
	done := make(chan events.Event, 1)
	go func() {
		done <- gr.waitForWakeup(result)
	}()

	// 同步点：确保订阅完成后再发布（竞态消除——发布在订阅完成前发生=事件丢失）。
	// 1s 宽限+5s 审批窗口——订阅建立最迟点距超时仍余 4s，wait_more 可达。
	time.Sleep(1 * time.Second)

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
	// 轮询窗口 10s——必须覆盖 waitForWakeup 的旧超时（5s 审批窗口）+ wait_more
	// 重挂后的完整窗口（5s），否则新快照（TimeoutAt=原值+5s）写入时轮询已耗尽。
	deadline := time.Now().Add(10 * time.Second)
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
	// waitForWakeup 应已收到 wait_more 决策并返回（延长+持久化发生在返回前）。
	evt := <-done
	if evt.Type != waitMoreDecisionEventType {
		t.Errorf("MUST 1 前置失败: waitForWakeup 未被 wait_more 决策唤醒——返回类型=%s", evt.Type)
	}
	if !extended {
		t.Errorf("MUST 1 失败（R-1376 wait_more 双写）: wait_more 决策后 PipelineState.TimeoutAt 未被延长——原值=%s 当前快照值=%s；无 wait_more 决策路径消费 UserDecisionReceived 事件", original, rewritten)
	}
}
