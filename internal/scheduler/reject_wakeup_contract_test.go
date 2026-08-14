// 契约测试：waitForWakeup(reason=approval) 唤醒集闭合（R-1376）。
//
// 断言来源: R-1376（Goal 级状态机闭合——拒绝族入唤醒集。07 事件注册表 §3：
//   "waitForWakeup(reason=approval) 的唤醒事件集=UserApprovedAction ∪ ActionRejected
//   ∪ ActionCancelled ∪ SecurityIncident(guard 族) ∪ GoalNeedsReview——拒绝族全部入集"）。
//
// 先红状态（2026-08-14）: wakeupEventForReason("approval") 当前仅返回
//   TypeUserApprovedAction（goalrunner.go）——ActionRejected / ActionCancelled /
//   SecurityIncident(guard 族) / GoalNeedsReview 均不在订阅集内。拒绝族事件发布后
//   无人消费 → waitForWakeup 阻塞至 TimeoutAt 超时并返回 WaitTimeout → 断言
//   "在超时前被拒绝族事件唤醒"失败 → 红。
//
// 转绿任务: 1.27/1.28（计划 C-2 表——R-1376 唤醒集闭合：waitForWakeup 订阅集并入
//   拒绝族四事件，并同步注册 events.TypeGoalNeedsReview 常量，wire 值保持
//   "GoalNeedsReview"）。
//
// 契约 MUST（R-1376）:
//   - MUST 1: reason=approval 等待期间，ActionRejected 事件必须唤醒 GoalRunner（超时前）。
//   - MUST 2: reason=approval 等待期间，ActionCancelled 事件必须唤醒 GoalRunner（超时前）。
//   - MUST 3: reason=approval 等待期间，SecurityIncident(guard 族) 事件必须唤醒 GoalRunner。
//   - MUST 4: reason=approval 等待期间，GoalNeedsReview 事件必须唤醒 GoalRunner（超时前）。
//
// 纪律: 禁止源码文本断言（check-anti-cheat R-568）——本测试仅行为断言：
// 发布唤醒事件 → 观察 waitForWakeup 是否在 TimeoutAt 之前返回对应事件。

package scheduler

import (
	"testing"
	"time"

	"github.com/goalos/goalos/internal/eventbus"
	"github.com/goalos/goalos/internal/statestore"
	"github.com/goalos/goalos/pkg/events"
)

// goalNeedsReviewEventType 是 GoalNeedsReview 的 wire 值（07 事件注册表 core 层注册，
// R-1219 S-24②/R-1376——NeedsReview 唯一入口事件）。
// pkg/events 尚未导出对应常量——转绿任务 1.27 应注册 events.TypeGoalNeedsReview 且
// 保持 wire 值不变；本测试以 wire 值字面量断言行为，不依赖常量是否存在。
const goalNeedsReviewEventType = "GoalNeedsReview"

func TestReject_WakesGoalRunner(t *testing.T) {
	// R-1376 唤醒集：approval 等待下，拒绝族全部入集。
	wakeupSet := []struct {
		name      string
		eventType string
	}{
		{"action_rejected", events.TypeActionRejected},
		{"action_cancelled", events.TypeActionCancelled},
		{"security_incident", events.TypeSecurityIncident},
		{"goal_needs_review", goalNeedsReviewEventType},
	}

	for _, tt := range wakeupSet {
		t.Run(tt.name, func(t *testing.T) {
			goalID := "goal-reject-wakeup-" + tt.name
			bus := eventbus.New()
			store := statestore.New(t.TempDir())
			pr := NewPipelineRunner(bus, store)
			gr := NewGoalRunner(Goal{ID: goalID}, bus, store, pr, NewGoalAnchorTracker(3))

			// 等待窗口：短超时 500ms——红态下 waitForWakeup 超时返回 WaitTimeout，
			// 蓝态（转绿后）下拒绝族事件在毫秒级唤醒，测试自身不引入长等待。
			result := &PipelineResult{
				Status:     PipelineWaiting,
				WaitReason: string(WaitApproval),
				PipelineState: &PipelineState{
					ResumePrimitive: ResumeFromWait,
					WaitReason:      string(WaitApproval),
					TimeoutAt:       time.Now().Add(500 * time.Millisecond).Format(time.RFC3339),
				},
			}

			done := make(chan events.Event, 1)
			go func() { done <- gr.waitForWakeup(result) }()

			// 发布拒绝族事件——契约要求：超时前必须唤醒 GoalRunner。
			bus.Publish(events.Event{
				Type:   tt.eventType,
				GoalID: goalID,
				Source: "test",
				Payload: map[string]interface{}{
					"action_id": "act-1",
					"module":    "guard", // SecurityIncident(guard 族)——07 X.8
					"severity":  "WARN",
				},
			})

			select {
			case got := <-done:
				if got.Type == "WaitTimeout" {
					t.Errorf("MUST 1-4 失败（R-1376 唤醒集闭合）: waitForWakeup(reason=approval) 未被 %s 在超时前唤醒——当前订阅集缺少拒绝族事件 %s", tt.eventType, tt.eventType)
				} else if got.Type != tt.eventType {
					t.Errorf("MUST 1-4 失败（R-1376）: waitForWakeup 被 %s 唤醒而非 %s——唤醒事件类型错误", got.Type, tt.eventType)
				}
			case <-time.After(3 * time.Second):
				t.Errorf("MUST 1-4 失败（R-1376 唤醒集闭合）: waitForWakeup(reason=approval) 对 %s 既未被唤醒也未超时——订阅集/超时计算异常", tt.eventType)
			}
		})
	}
}
