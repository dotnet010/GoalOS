// 契约测试：Approval 否定语义必须携带进 AI 上下文与用户呈现（R-1400，会议 #205 G-4）。
//
// 断言来源: R-1400（AI 上下文语义保真契约——会议 #205 G-4）:
//   "GoalAgent 重规划/复盘输入与 Persona 用户呈现必须携带 reason 字段——approval_timeout
//   （系统客观超时→用户未响应）与 user_rejected（人类主观否定→方案需改）在 AI 上下文中
//   显式区分；禁止仅凭终态 Rejected 推断否定语义。"
//
// 包位置说明: 契约的主锚点（AI 上下文构造入口）在 missionengine（PlanRequested →
//   Agent.Context 管线——重规划/复盘复用同一管线），故测试置于 internal/missionengine/；
//   Persona 用户呈现锚点位于 internal/persona（可被 internal 测试导入，行为探针跨包调用）。
//
// 先红状态（2026-08-14）: 当前代码无任何 reason 传递路径——
//   - missionengine.Context 仅有 GoalID/GoalText/AnchorCheck 三字段，无 reason 字段；
//     Engine.handlePlanRequested 只读取 payload 的 goal_text / goal_anchor_check /
//     flow_name，reason 载荷键被静默丢弃（探针 A1 红——AI 上下文不携带原 reason）。
//   - persona.Concise.Render 对 ActionRejected 恒返回 "已拒绝。"，不读取任何 reason/
//     reject_reason 载荷键；Warm/Minimal 甚至无 ActionRejected 分支（探针 A2 红——
//     两种否定语义呈现不可区分）。
//   - missionengine/persona 公开类型中无 Reason 相关字段或方法（探针 B 红——
//     reflect 枚举零命中）。
//
// 转绿任务: 3.26（C-2 表——R-1400 AI 上下文语义保真：Context 增加 reason 传递字段，
//   handlePlanRequested 透传 PlanRequested 载荷中的 reason；Persona ActionRejected
//   呈现区分 approval_timeout 与 user_rejected）。
//
// 契约 MUST（R-1400）:
//   - MUST 1: 重规划/复盘输入（PlanRequested→Agent.Context）必须携带原否定语义 reason——
//     approval_timeout 与 user_rejected 可区分恢复，禁止静默丢弃。
//   - MUST 2: Persona 用户呈现（ActionRejected）必须区分两种否定语义——禁止对
//     approval_timeout 与 user_rejected 渲染相同文本（用户无法分辨"系统超时未响应"
//     与"人类主动否定"）。
//   - MUST 3: missionengine/persona 公开类型必须存在 Reason 传递路径（字段或方法）——
//     禁止仅凭终态 Rejected 推断否定语义。
//
// 纪律: 编译安全探针（行为断言 + reflect 枚举）——禁止读源码文本断言（check-anti-cheat
//   R-568）。逐条 t.Errorf 而非 FailNow——一次性报告全部缺口。

package missionengine_test

import (
	"fmt"
	"reflect"
	"strings"
	"sync"
	"testing"

	"github.com/goalos/goalos/internal/eventbus"
	"github.com/goalos/goalos/internal/missionengine"
	"github.com/goalos/goalos/internal/persona"
	"github.com/goalos/goalos/pkg/events"
)

// 两种否定语义的 reason 值（R-1400 契约枚举）。
// 与系统既有产出同源：approval_timeout=governance handleApprovalTimeout 的
// reject_reason（R-1384）；user_rejected=daemon HandleReject 的 reason。
const (
	reasonApprovalTimeout = "approval_timeout" // 系统客观超时→用户未响应
	reasonUserRejected    = "user_rejected"    // 人类主观否定→方案需改
)

// goalNeedsReviewWire 是 GoalNeedsReview 的 wire 值（07 事件注册表 core 层注册，
// R-1219 S-24②/R-1376——NeedsReview 唯一入口事件）。pkg/events 尚未导出对应常量
// （转绿任务 1.27 注册）——本测试以 wire 值字面量发布语义来源事件，不依赖常量。
const goalNeedsReviewWire = "GoalNeedsReview"

// captureAgent 是探针 A1 的捕获桩——实现公开 Agent 接口，记录每次规划调用收到的
// AI 上下文（Align/Analyze/Plan 三阶段同 Context 值）。这是行为断言：
// 输入事件携带 reason → 观察 Agent 收到的上下文是否携带该 reason。
type captureAgent struct {
	mu       sync.Mutex
	contexts []missionengine.Context
}

func (c *captureAgent) Align(goal string, ctx missionengine.Context) (*missionengine.CompletionCriteria, error) {
	c.mu.Lock()
	c.contexts = append(c.contexts, ctx)
	c.mu.Unlock()
	return &missionengine.CompletionCriteria{
		GoalID: ctx.GoalID, SuccessDefinition: goal, Complexity: "low",
	}, nil
}

func (c *captureAgent) Analyze(criteria *missionengine.CompletionCriteria, ctx missionengine.Context) (*missionengine.TaskAnalysis, error) {
	c.mu.Lock()
	c.contexts = append(c.contexts, ctx)
	c.mu.Unlock()
	return &missionengine.TaskAnalysis{
		GoalID: ctx.GoalID, Complexity: "low", SuggestedFlow: "generic-v1",
		RiskAssessment: "L0", EstimatedSteps: 1,
	}, nil
}

func (c *captureAgent) Plan(criteria *missionengine.CompletionCriteria, analysis *missionengine.TaskAnalysis, flowName string, ctx missionengine.Context) (*missionengine.MissionGraph, error) {
	c.mu.Lock()
	c.contexts = append(c.contexts, ctx)
	c.mu.Unlock()
	return &missionengine.MissionGraph{
		GoalID: ctx.GoalID,
		Nodes:  []missionengine.GraphNode{{ID: "1", Type: "mission", Description: ctx.GoalText, ActionType: "fs.read", Target: "reason-contract"}},
	}, nil
}

func (c *captureAgent) Verify(code string, actionID string, ctx missionengine.Context) (*missionengine.VerificationResult, error) {
	return &missionengine.VerificationResult{ActionID: actionID, Verdict: "PASS", Reason: "capture-agent", Score: 100}, nil
}

// TestApproval_ReasonPreservedInAgentContext — 否定语义必须携带进 AI 上下文（R-1400）。
//
// MUST 1 重规划/复盘输入（PlanRequested→Agent.Context）携带原 reason，两值可区分恢复
// MUST 2 Persona 呈现区分 approval_timeout 与 user_rejected（禁止同文本）
// MUST 3 公开类型存在 Reason 传递路径（反射枚举，禁止终态推断）
func TestApproval_ReasonPreservedInAgentContext(t *testing.T) {
	gaps := 0

	// ── 探针 A1（行为）: AI 上下文构造入口（PlanRequested→Agent.Context）──
	// 输入构造：先发布语义来源事件 GoalNeedsReview(reason=...)，再经重规划共用管线
	// PlanRequested(reason=...) 驱动规划——捕获 Agent 收到的上下文，断言原 reason
	// 未被丢弃（R-1400 MUST 1）。
	// 红锚：handlePlanRequested 仅读取 goal_text/goal_anchor_check/flow_name，
	// reason 载荷键被静默丢弃；Context 无 reason 字段 → 捕获上下文不含原 reason。
	foundReasons := map[string]bool{}
	for i, tc := range []struct {
		name   string
		reason string
	}{
		{"approval_timeout", reasonApprovalTimeout},
		{"user_rejected", reasonUserRejected},
	} {
		t.Run("context_"+tc.name, func(t *testing.T) {
			// 探针 A1 反误报纪律：goalID 与 goal_text 必须中性——不得含 reason 值
			// 本身，否则 Contains 断言会在"reason 未传递"时误判绿。唯一差异源 =
			// PlanRequested 载荷中的 reason 键。
			goalID := fmt.Sprintf("goal-reason-%02d", i+1)
			bus := eventbus.New()
			cap := &captureAgent{}
			engine := missionengine.New(bus, cap)
			engine.Start()

			// 语义来源：拒绝族事件携带否定语义（GoalNeedsReview——R-1219 S-24②）。
			bus.Publish(events.Event{
				Type:   goalNeedsReviewWire,
				GoalID: goalID,
				Source: "test",
				Payload: map[string]interface{}{
					"reason": tc.reason,
				},
			})

			// AI 输入构造入口：重规划/复盘输入经 PlanRequested 进入 Agent.Context
			// 管线——reason 载荷键是契约载体（R-1400 转绿任务 3.26 必须透传）。
			bus.Publish(events.Event{
				Type:   events.TypePlanRequested,
				GoalID: goalID,
				Source: "scheduler",
				Payload: map[string]interface{}{
					"goal_text": "中性目标文本（不含任何 reason 值——探针 A1 唯一差异源是 reason 键）",
					"reason":    tc.reason,
				},
			})

			cap.mu.Lock()
			contexts := append([]missionengine.Context(nil), cap.contexts...)
			cap.mu.Unlock()

			if len(contexts) == 0 {
				t.Errorf("MUST 1（R-1400）: 探针 A1 无捕获上下文——规划管线未到达 Agent（goal=%s），无法断言 reason 传递", goalID)
				gaps++
				return
			}

			preserved := false
			for _, ctx := range contexts {
				if strings.Contains(fmt.Sprintf("%+v", ctx), tc.reason) {
					preserved = true
				}
			}
			if !preserved {
				t.Errorf("MUST 1（R-1400）: AI 上下文未携带原否定语义 %q（goal=%s）——捕获上下文集=%v；handlePlanRequested 丢弃 reason 载荷键且 Context 无 reason 字段，禁止仅凭终态 Rejected 推断否定语义（转绿任务 3.26）", tc.reason, goalID, contexts)
				gaps++
				return
			}
			foundReasons[tc.reason] = true
		})
	}
	// 可区分性闭合检查：两种否定语义必须都能从 AI 上下文恢复（R-1400——显式区分，
	// 非终态推断）。任一缺失→红。
	if len(foundReasons) != 2 {
		t.Errorf("MUST 1（R-1400）: 两种否定语义未在 AI 上下文中可区分恢复——approval_timeout 与 user_rejected 必须各自携带，实际可区分集合=%v", foundReasons)
		gaps++
	}

	// ── 探针 A2（行为）: Persona 用户呈现区分两种否定语义（R-1400 MUST 2）──
	// 输入构造：ActionRejected 呈现载荷（reason/reject_reason 两种既有键——
	// governance 用 reject_reason、daemon 用 reason）；断言两种原因渲染结果不同且非空。
	// 红锚：Concise.Render 对 ActionRejected 恒返回 "已拒绝。"，不读取任何载荷键。
	distinguished := 0
	for _, key := range []string{"reason", "reject_reason"} {
		outTimeout := persona.Concise.Render("ActionRejected", map[string]interface{}{key: reasonApprovalTimeout})
		outRejected := persona.Concise.Render("ActionRejected", map[string]interface{}{key: reasonUserRejected})
		if outTimeout != "" && outRejected != "" && outTimeout != outRejected {
			distinguished++
		}
	}
	if distinguished == 0 {
		t.Errorf("MUST 2（R-1400）: Persona(concise) 对 ActionRejected 的两种否定语义呈现不可区分（approval_timeout=%q 与 user_rejected=%q 渲染相同文本）——用户无法分辨'系统超时未响应'与'人类主动否定'；Persona 呈现必须携带并区分 reason（转绿任务 3.26）", persona.Concise.Render("ActionRejected", map[string]interface{}{"reason": reasonApprovalTimeout}), persona.Concise.Render("ActionRejected", map[string]interface{}{"reason": reasonUserRejected}))
		gaps++
	}

	// ── 探针 B（反射）: 公开类型 Reason 传递路径必须存在（R-1400 MUST 3）──
	// reflect 枚举（编译安全——不引用不存在的字段/方法名）：
	// missionengine.Context 字段、Engine/GoalAgent 方法、persona.Persona 字段。
	// 红锚：Context={GoalID,GoalText,AnchorCheck}、Persona={Name,Description,
	// AckWord,DoneWord,WarnWord,Render}，Engine/GoalAgent 方法均无 reason 相关名。
	reasonPath := false
	for _, typ := range []reflect.Type{
		reflect.TypeOf(missionengine.Context{}),
		reflect.TypeOf(persona.Persona{}),
	} {
		for i := 0; i < typ.NumField(); i++ {
			if strings.Contains(strings.ToLower(typ.Field(i).Name), "reason") {
				reasonPath = true
			}
		}
	}
	for _, typ := range []reflect.Type{
		reflect.TypeOf(&missionengine.Engine{}),
		reflect.TypeOf(&missionengine.GoalAgent{}),
	} {
		for i := 0; i < typ.NumMethod(); i++ {
			name := strings.ToLower(typ.Method(i).Name)
			if strings.Contains(name, "reason") || strings.Contains(name, "replan") || strings.Contains(name, "review") {
				reasonPath = true
			}
		}
	}
	if !reasonPath {
		t.Errorf("MUST 3（R-1400）: missionengine/persona 公开类型无 Reason 传递路径（Context 无 reason 字段、Engine/GoalAgent 无相关入口、Persona 无 reason 呈现载体）——重规划/复盘输入与用户呈现无法携带否定语义，禁止仅凭终态 Rejected 推断（转绿任务 3.26）")
		gaps++
	}

	if gaps > 0 {
		t.Errorf("R-1400 契约缺口 %d 项——Approval 否定语义（approval_timeout/user_rejected）未显式区分进入 AI 上下文与用户呈现（会议 #205 G-4）", gaps)
	}
}
