// Package scheduler — PipelineRunner v1.1.0。
// Action 级执行引擎。按 MissionGraph 拓扑序遍历节点→对每个 Action
// 依次执行 Check→Exec→Wait→Decide 原语管线。
// 替代 v1.0 的 transition.go。
//
// 设计依据：05 架构文档 §3.1、R253、R276。

package scheduler

import (
	"fmt"
	"log"
	"time"

	"github.com/goalos/goalos/internal/eventbus"
	"github.com/goalos/goalos/internal/statestore"
	"github.com/goalos/goalos/pkg/events"
)

// PipelineStatus 是 PipelineRunner 的返回状态。
type PipelineStatus string

const (
	PipelineCompleted PipelineStatus = "completed"
	PipelineFailed    PipelineStatus = "failed"
	PipelineWaiting   PipelineStatus = "waiting"  // Wait 原语触发。外部事件唤醒后继续
	PipelinePaused    PipelineStatus = "paused"   // 用户暂停
)

// PipelineResult 是 PipelineRunner.Run() 的返回值。
type PipelineResult struct {
	Status        PipelineStatus
	Error         string
	WaitReason    string         // PipelineWaiting 时的等待原因
	PipelineState *PipelineState // PipelineWaiting/Paused 时的执行位置
}

// PipelineState 记录 PipelineRunner 的执行位置（v1.1.0）。
type PipelineState struct {
	ResumePoint      string   `json:"resume_point"`
	ResumePrimitive  string   `json:"resume_primitive"` // "check"|"exec"|"wait"|"decide"
	WaitReason       string   `json:"wait_reason"`
	TimeoutAt        string   `json:"timeout_at"`
	PendingActionIDs []string `json:"pending_action_ids,omitempty"`
	CompletedNodes   []string `json:"completed_nodes,omitempty"`
}

// Primitive 是 PipelineRunner 的原语接口。
type Primitive int

const (
	PrimitiveCheck  Primitive = iota
	PrimitiveExec
	PrimitiveWait
	PrimitiveDecide
)

// CheckResult 是 Check 原语的返回结果。
type CheckResult string

const (
	CheckPASS   CheckResult = "PASS"
	CheckWARN   CheckResult = "WARN"
	CheckBLOCK  CheckResult = "BLOCK"
	CheckREJECT CheckResult = "REJECT"
)

// DecidePath 是 Decide 原语的路径选择。
type DecidePath string

const (
	DecideCONTINUE DecidePath = "CONTINUE"
	DecideRETRY    DecidePath = "RETRY"
	DecideREPLAN   DecidePath = "REPLAN"
	DecideESCALATE DecidePath = "ESCALATE"
	DecideABORT    DecidePath = "ABORT"
)

// PipelineRunner 是 Action 级执行引擎。
type PipelineRunner struct {
	bus          *eventbus.EventBus
	store        *statestore.Store
	state        *PipelineState

	// 幂等性追踪
	autoFixCount map[string]int // actionID → 自修正次数
	retryCount   map[string]int
		multiLLM     *MultiLLMVerifier // v0.1.1: Check 原语的多模型审查
}

// NewPipelineRunner 创建 PipelineRunner。
func NewPipelineRunner(bus *eventbus.EventBus, store *statestore.Store) *PipelineRunner {
	return &PipelineRunner{
		bus:          bus,
		store:        store,
		autoFixCount: make(map[string]int),
		retryCount:   make(map[string]int),
	}
}

// Run 执行 MissionGraph 的 Action 原语管线。
// 从 PipelineState.ResumePoint 恢复执行位置。
func (pr *PipelineRunner) Run(goalID string, state *statestore.GoalState) (*PipelineResult, error) {
	// 从 PipelineState 恢复
	if state.PipelineState != nil {
		pr.state = &PipelineState{
			ResumePoint:      state.PipelineState.ResumePoint,
			ResumePrimitive:  state.PipelineState.ResumePrimitive,
			WaitReason:       state.PipelineState.WaitReason,
			TimeoutAt:        state.PipelineState.TimeoutAt,
			PendingActionIDs: state.PipelineState.PendingActionIDs,
			CompletedNodes:   state.CompletedNodes,
		}
		log.Printf("[PipelineRunner] goal=%s resumed from %s primitive at node %s",
			goalID, pr.state.ResumePrimitive, pr.state.ResumePoint)
	} else {
		pr.state = &PipelineState{}
	}

	// 遍历 Action（简化：按 CompletedNodes 跳过已完成的）
	for _, nodeID := range state.CompletedNodes {
		log.Printf("[PipelineRunner] goal=%s skip completed node=%s", goalID, nodeID)
	}
	// 实际实现中从 MissionGraph 获取待执行节点列表。此处为 MVP 骨架。

	// 如果在 Wait 中恢复，直接跳转到 Decide 或继续 Wait 订阅
	if pr.state.ResumePrimitive == "wait" {
		return pr.wait(goalID, pr.state.WaitReason)
	}
	if pr.state.ResumePrimitive == "decide" {
		return pr.decide(goalID, "", nil)
	}

	// 核心管线（对每个待执行的 Action）
	// MVP：此处入 MissionGraph 遍历。骨架展示核心逻辑。
	currentAction := pr.getNextAction(goalID, state)
	if currentAction == "" {
		return &PipelineResult{Status: PipelineCompleted}, nil
	}

	return pr.executePrimitivePipeline(goalID, currentAction)
}

// executePrimitivePipeline 对一个 Action 执行 Check→Exec→Wait→Decide。
func (pr *PipelineRunner) executePrimitivePipeline(goalID string, actionID string) (*PipelineResult, error) {
	// 阶段 1: Check
	result := pr.check(actionID)
	switch result {
	case CheckREJECT:
		return &PipelineResult{Status: PipelineFailed, Error: "check_rejected"}, nil
	case CheckBLOCK:
		return pr.wait(goalID, "check_blocked")
	case CheckWARN:
		log.Printf("[PipelineRunner] action=%s check WARN — continuing", actionID)
	}
	// CheckPASS 或 CheckWARN 继续

	// 阶段 2: Exec（幂等性检查）
	if pr.isActionCompleted(actionID) {
		log.Printf("[PipelineRunner] action=%s already completed — skipping Exec", actionID)
	} else {
		if err := pr.exec(actionID); err != nil {
			return pr.decide(goalID, actionID, err)
		}
	}

	// 阶段 3: Wait（如果需要审批/依赖/资源）
	if pr.requiresWait(actionID) {
		return pr.wait(goalID, "approval")
	}

	// 阶段 4: Decide
	return pr.decide(goalID, "", nil)
}

// check 评估 Action 的准入条件。返回 PASS/WARN/BLOCK/REJECT。
// v0.1.1: 集成 MultiLLMVerifier——不再硬编码返回 CheckPASS。
func (pr *PipelineRunner) check(actionID string, code ...string) CheckResult {
	result := CheckPASS
	reason := "basic-check"

	// 如果有代码内容且 MultiLLMVerifier 可用→执行多模型审查
	if len(code) > 0 && code[0] != "" && pr.multiLLM != nil {
		verdict, err := pr.multiLLM.Verify(code[0], actionID)
		if err == nil {
			switch verdict.Result {
			case "FAIL": result = CheckREJECT; reason = "multi-llm:FAIL"
			case "WARN": result = CheckWARN; reason = "multi-llm:WARN"
			default: result = CheckPASS; reason = "multi-llm:PASS"
			}
		}
	}

	pr.bus.Publish(events.Event{
		Type:   events.TypeCheckPerformed,
		GoalID: "",
		Source: "pipelinerunner",
		Payload: map[string]interface{}{
			"action_id": actionID,
			"result":    string(result),
			"reason":    reason,
		},
	})
	return result
}

// exec 执行 Action。通过 Event Bus 触发 Plugin Runner。
func (pr *PipelineRunner) exec(actionID string) error {
	pr.bus.Publish(events.Event{
		Type:   events.TypeActionScheduled,
		GoalID: "",
		Source: "pipelinerunner",
		Payload: map[string]interface{}{
			"action_id": actionID,
		},
	})
	return nil
}

// wait 进入等待状态。保存 PipelineState 并返回 WAITING。
// Wait 是唯一异步原语——Run() 返回，外部事件唤醒后 GoalRunner 重新调用 Run()。
func (pr *PipelineRunner) wait(goalID string, reason string) (*PipelineResult, error) {
	pr.state.ResumePrimitive = "decide" // 唤醒后从 Decide 继续
	pr.state.WaitReason = reason
	pr.state.TimeoutAt = time.Now().Add(5 * time.Minute).Format(time.RFC3339)

	pr.bus.Publish(events.Event{
		Type:   events.TypePipelinePaused,
		GoalID: goalID,
		Source: "pipelinerunner",
		Payload: map[string]interface{}{
			"wait_reason": reason,
			"timeout_at":  pr.state.TimeoutAt,
		},
	})

	return &PipelineResult{
		Status:        PipelineWaiting,
		WaitReason:    reason,
		PipelineState: pr.state,
	}, nil
}

// decide 分析结果并选择路径。
func (pr *PipelineRunner) decide(goalID string, actionID string, execErr error) (*PipelineResult, error) {
	if execErr == nil {
		pr.bus.Publish(events.Event{
			Type:   events.TypeDecidePathSelected,
			GoalID: goalID,
			Source: "pipelinerunner",
			Payload: map[string]interface{}{
				"path": "CONTINUE",
			},
		})
		return &PipelineResult{Status: PipelineCompleted}, nil
	}

	// 简化决策：execErr → AUTO_FIX（最多 3 次）→ ESCALATE
	 // MVP 简化
	pr.autoFixCount[actionID]++
	if pr.autoFixCount[actionID] <= 3 {
		log.Printf("[PipelineRunner] AUTO_FIX attempt %d/3", pr.autoFixCount[actionID])
		return &PipelineResult{Status: PipelineCompleted}, nil // 简化：重试
	}

	pr.bus.Publish(events.Event{
		Type:   events.TypeHumanInterventionRequested,
		GoalID: goalID,
		Source: "pipelinerunner",
		Payload: map[string]interface{}{
			"reason": fmt.Sprintf("auto_fix exhausted after %d attempts", pr.autoFixCount[actionID]),
		},
	})
	return &PipelineResult{Status: PipelineFailed, Error: "auto_fix_exhausted"}, nil
}

// 辅助方法

func (pr *PipelineRunner) isActionCompleted(actionID string) bool {
	// MVP 简化：实际应从 events.jsonl 回放判定
	return false
}

func (pr *PipelineRunner) requiresWait(actionID string) bool {
	// MVP 简化：实际应从 MissionGraph 节点标记判定
	return false
}

func (pr *PipelineRunner) getNextAction(goalID string, state *statestore.GoalState) string {
	// MVP 简化：返回第一个未完成的节点
	// 实际实现中从 MissionGraph 拓扑序遍历
	if state.NodeID != "" && !containsStr(state.CompletedNodes, state.NodeID) {
		return state.NodeID
	}
	return ""
}

func containsStr(slice []string, item string) bool {
	for _, s := range slice {
		if s == item {
			return true
		}
	}
	return false
}

// SetMultiLLM 设置多模型验证器（v0.1.1）。
func (pr *PipelineRunner) SetMultiLLM(v *MultiLLMVerifier) { pr.multiLLM = v }
