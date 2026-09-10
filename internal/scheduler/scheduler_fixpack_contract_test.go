// scheduler_fixpack_contract_test.go——会议 #247 scheduler 修正包先红（R-1594/R-1595；
// R-1625 编号化=任务 3.5/3.6；Beck 纪律——W3 周一先红/W3-4 转绿；12 清单 G 节两行）。
// 纪律: 禁止源码文本断言（check-anti-cheat R-568）——本测试仅行为断言。
package scheduler

import (
	"testing"
	"time"

	"github.com/goalos/goalos/internal/eventbus"
	"github.com/goalos/goalos/internal/statestore"
	"github.com/goalos/goalos/pkg/events"
)

// TestWakeupSet_PostExec_CompletionWakes（12 清单 G 节——R-1594）：
// post_exec 等待必须被 ActionCompleted/ActionFailed/ActionCancelled 在超时前唤醒
// （05 §3.3 等待条件→唤醒事件映射表权威行——R-1594 会议 #247 修正 post_exec 原落
// default 仅 GoalResumed 的缺口）。
// 先红状态（2026-08-28 W3 周一）: wakeupEventSetForReason("post_exec") 当前落 default
// 分支（goalrunner.go）——仅订阅 GoalResumed，ActionCompleted 发布后不唤醒→超时判红。
// 转绿任务: 3.5——post_exec 唤醒集=ActionCompleted∪ActionFailed∪ActionCancelled∪GoalResumed。
func TestWakeupSet_PostExec_CompletionWakes(t *testing.T) {
	wakeSet := []struct {
		name      string
		eventType string
	}{
		{"action_completed", events.TypeActionCompleted},
		{"action_failed", events.TypeActionFailed},
		{"action_cancelled", events.TypeActionCancelled},
	}
	for _, tt := range wakeSet {
		t.Run(tt.name, func(t *testing.T) {
			goalID := "goal-postexec-" + tt.name
			bus := eventbus.New()
			store := statestore.New(t.TempDir())
			pr := NewPipelineRunner(bus, store)
			gr := NewGoalRunner(Goal{ID: goalID}, bus, store, pr, NewGoalAnchorTracker(3))

			result := &PipelineResult{
				Status:     PipelineWaiting,
				WaitReason: "post_exec",
				PipelineState: &statestore.PipelineSnapshot{
					ResumePrimitive: string(ResumeFromWait),
					WaitReason:      "post_exec",
					TimeoutAt:       time.Now().Add(5 * time.Second).Format(time.RFC3339),
				},
			}

			done := make(chan events.Event, 1)
			go func() { done <- gr.waitForWakeup(result) }()

			// 同步点（同 reject_wakeup 契约测试先例）：订阅建立需数十 ms。
			time.Sleep(200 * time.Millisecond)

			bus.Publish(events.Event{
				Type:   tt.eventType,
				GoalID: goalID,
				Source: "test",
				Payload: map[string]interface{}{
					"action_id": "act-1",
				},
			})

			select {
			case got := <-done:
				if got.Type == "WaitTimeout" {
					t.Errorf("R-1594 失败: waitForWakeup(reason=post_exec) 未被 %s 在超时前唤醒——post_exec 唤醒集缺口未闭合", tt.eventType)
				} else if got.Type != tt.eventType {
					t.Errorf("R-1594 失败: 被 %s 唤醒而非 %s", got.Type, tt.eventType)
				}
			case <-time.After(3 * time.Second):
				t.Errorf("R-1594 失败: 对 %s 既未唤醒也未超时", tt.eventType)
			}
		})
	}
}

// TestMultiLLM_Synthesis_TimeoutQuorum（12 清单 G 节——R-1595 五规则合成函数对齐）：
// 四行矩阵（TIMEOUT 票=采集层重试 1 次退避 5s 后的最终票值——合成层只见最终票）：
// (1)1 TIMEOUT+2 PASS→PASS+degraded=true（TIMEOUT 独立票值不计 FAIL——自动化不被网络抖动打断）
// (2)1 FAIL+2 PASS→DIVERGENCE（人工裁定）
// (3)全 FAIL→FAIL
// (4)有效票<quorum(2)（2 TIMEOUT+1 PASS）→quorum 不足→NeedsReview(verification_quorum_unmet)——Kees 加固：不静默吞票
// 先红状态（2026-08-28 W3 周一）: Combine 当前=加权评分（无 DIVERGENCE/TIMEOUT/quorum 语义——
// R-1250 S-20 四规则规格与代码双侧失锚的代码侧）——矩阵全红。
// 转绿任务: 3.6——Combine 五规则重写。
func TestMultiLLM_Synthesis_TimeoutQuorum(t *testing.T) {
	vc := &VerdictCombiner{}
	vote := func(v string) ProviderVote { return ProviderVote{Provider: "p", Model: "m", Vote: v} }

	// (1)1 TIMEOUT+2 PASS → PASS+degraded=true
	v1 := vc.Combine([]ProviderVote{vote("TIMEOUT"), vote("PASS"), vote("PASS")})
	if v1.Result != "PASS" || !v1.Degraded {
		t.Errorf("(1)失败: 1 TIMEOUT+2 PASS 应=PASS+degraded=true，实际 result=%s degraded=%v", v1.Result, v1.Degraded)
	}
	// (2)1 FAIL+2 PASS → DIVERGENCE
	v2 := vc.Combine([]ProviderVote{vote("FAIL"), vote("PASS"), vote("PASS")})
	if v2.Result != "DIVERGENCE" {
		t.Errorf("(2)失败: 1 FAIL+2 PASS 应=DIVERGENCE，实际 %s", v2.Result)
	}
	// (3)全 FAIL → FAIL
	v3 := vc.Combine([]ProviderVote{vote("FAIL"), vote("FAIL"), vote("FAIL")})
	if v3.Result != "FAIL" {
		t.Errorf("(3)失败: 全 FAIL 应=FAIL，实际 %s", v3.Result)
	}
	// (4)有效票<quorum(2) → quorum 不足
	v4 := vc.Combine([]ProviderVote{vote("TIMEOUT"), vote("TIMEOUT"), vote("PASS")})
	if !v4.QuorumUnmet {
		t.Errorf("(4)失败: 有效票 1<quorum 2 应=QuorumUnmet=true（→NeedsReview(verification_quorum_unmet)），实际 result=%s quorum_unmet=%v", v4.Result, v4.QuorumUnmet)
	}
}
