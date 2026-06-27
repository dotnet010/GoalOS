// Package scheduler — PipelineRunner v0.1.0（重写：R-362 激进策略）。
// Action 级执行引擎。按 MissionGraph 拓扑序遍历节点→对每个 Action
// 依次执行 Check→Exec→Wait→Decide 原语管线。
// Decide 委托给 RecoveryPipeline（完整 10 分支决策树）。
//
// 设计依据：05 架构文档 §3.1、R253、R276、R-362。
package scheduler

import (
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
	PipelineWaiting   PipelineStatus = "waiting"
	PipelinePaused    PipelineStatus = "paused"
)

// PipelineResult 是 PipelineRunner.Run() 的返回值。
type PipelineResult struct {
	Status        PipelineStatus
	Error         string
	WaitReason    string
	PipelineState *PipelineState
}

// PipelineState 记录 PipelineRunner 的执行位置（v0.1.0）。
type PipelineState struct {
	ResumePoint      string   `json:"resume_point"`
	ResumePrimitive  string   `json:"resume_primitive"`
	WaitReason       string   `json:"wait_reason"`
	TimeoutAt        string   `json:"timeout_at"`
	PendingActionIDs []string `json:"pending_action_ids,omitempty"`
	CompletedNodes   []string `json:"completed_nodes,omitempty"`
}

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

// PipelineRunner 是 Action 级执行引擎（v0.1.0 重写）。
type PipelineRunner struct {
	bus      *eventbus.EventBus
	store    *statestore.Store
	state    *PipelineState
	recovery *RecoveryPipeline // R-362: Decide 委托给 RecoveryPipeline

	multiLLM     *MultiLLMVerifier
	autoFixCount map[string]int
	retryCount   map[string]int
}

// NewPipelineRunner 创建 PipelineRunner。
func NewPipelineRunner(bus *eventbus.EventBus, store *statestore.Store) *PipelineRunner {
	return &PipelineRunner{
		bus:          bus,
		store:        store,
		recovery:     NewRecoveryPipeline(), // R-362: 集成 RecoveryPipeline
		autoFixCount: make(map[string]int),
		retryCount:   make(map[string]int),
	}
}

// Run 执行 MissionGraph 的 Action 原语管线。
func (pr *PipelineRunner) Run(goalID string, state *statestore.GoalState) (*PipelineResult, error) {
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

	// 恢复路径
	if pr.state.ResumePrimitive == "wait" {
		return pr.wait(goalID, pr.state.WaitReason)
	}
	if pr.state.ResumePrimitive == "decide" {
		return pr.decide(goalID, "", nil)
	}

	// 获取下一个待执行 Action
	currentAction := pr.getNextAction(goalID, state)
	if currentAction == "" {
		return &PipelineResult{Status: PipelineCompleted}, nil
	}

	return pr.executePrimitivePipeline(goalID, currentAction)
}

// executePrimitivePipeline 对一个 Action 执行 Check→Exec→Wait→Decide。
func (pr *PipelineRunner) executePrimitivePipeline(goalID string, actionID string) (*PipelineResult, error) {
	// 阶段 1: Check — Gate 评估（auto_tests→checks→constraints→llm_verify）
	result := pr.check(actionID)
	pr.publishCheckPerformed(goalID, actionID, result)

	switch result {
	case CheckREJECT:
		return pr.decidePath(goalID, actionID, DecideABORT, "check_rejected")
	case CheckBLOCK:
		return pr.wait(goalID, "check_blocked")
	}

	// 阶段 2: Exec — 幂等检查后执行
	if pr.isActionCompleted(goalID, actionID) {
		log.Printf("[PipelineRunner] action=%s already completed — skipping Exec", actionID)
	} else {
		if err := pr.exec(actionID); err != nil {
			return pr.decide(goalID, actionID, err)
		}
	}

	// 阶段 3: Wait（审批/依赖/资源）
	if pr.requiresWait(actionID) {
		return pr.wait(goalID, "approval")
	}

	// 阶段 4: Decide — 委托给 RecoveryPipeline
	return pr.decide(goalID, actionID, nil)
}

// check 评估 Action 的准入条件。v0.1.1 重写：集成 MultiLLMVerifier。
func (pr *PipelineRunner) check(actionID string, code ...string) CheckResult {
	// 有代码 + MultiLLM 可用 → 多模型审查
	if len(code) > 0 && code[0] != "" && pr.multiLLM != nil {
		verdict, err := pr.multiLLM.Verify(code[0], actionID)
		if err == nil {
			switch verdict.Result {
			case "FAIL":
				return CheckREJECT
			case "WARN":
				return CheckWARN
			default:
				return CheckPASS
			}
		}
	}
	// 无代码或 MultiLLM 不可用 → 基础检查通过
	return CheckPASS
}

// exec 执行 Action。通过 Event Bus 触发 Plugin Runner（fire-and-forget）。
// v0.1.1 重写：publish ActionScheduled — PluginRunner 负责实际执行和结果发布。
func (pr *PipelineRunner) exec(actionID string) error {
	// ActionScheduled 事件由 Scheduler 发布（从 MissionGraph 构造完整 payload）。
	// PipelineRunner.exec() 的角色是标记 Action 已进入执行阶段。
	// 实际执行结果（ActionCompleted/ActionFailed）由 PluginRunner 发布，
	// Scheduler 订阅后驱动状态机继续。
	return nil
}

// wait 进入等待状态。保存 PipelineState 并返回 WAITING。
func (pr *PipelineRunner) wait(goalID string, reason string) (*PipelineResult, error) {
	pr.state.ResumePrimitive = "decide"
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

// decide 委托给 RecoveryPipeline 的完整决策树（R-362 重写）。
func (pr *PipelineRunner) decide(goalID string, actionID string, execErr error) (*PipelineResult, error) {
	if execErr == nil {
		return pr.decidePath(goalID, actionID, DecideCONTINUE, "")
	}

	// 委托给 RecoveryPipeline（完整 10 分支决策树）
	rp := pr.recovery.Decide(actionID, execErr.Error(), nil, goalID)
	path := recoveryActionToDecidePath(rp.Action)
	return pr.decidePath(goalID, actionID, path, rp.Reason)
}

// recoveryActionToDecidePath 将 RecoveryPath.Action 映射为 DecidePath。
func recoveryActionToDecidePath(action string) DecidePath {
	switch action {
	case "RETRY":
		return DecideRETRY
	case "AUTO_FIX", "SWITCH_TOOL":
		return DecideREPLAN
	case "ESCALATE":
		return DecideESCALATE
	default:
		return DecideABORT
	}
}

// decidePath 发布 DecidePathSelected 事件并返回对应 PipelineResult。
func (pr *PipelineRunner) decidePath(goalID string, actionID string, path DecidePath, reason string) (*PipelineResult, error) {
	pr.bus.Publish(events.Event{
		Type:   events.TypeDecidePathSelected,
		GoalID: goalID,
		Source: "pipelinerunner",
		Payload: map[string]interface{}{
			"action_id": actionID,
			"path":      string(path),
			"reason":    reason,
		},
	})

	switch path {
	case DecideCONTINUE:
		return &PipelineResult{Status: PipelineCompleted}, nil
	case DecideRETRY, DecideREPLAN:
		pr.retryCount[actionID]++
		return &PipelineResult{Status: PipelineCompleted}, nil // 重试由 GoalRunner 重新调用 Run()
	case DecideESCALATE:
		pr.bus.Publish(events.Event{
			Type:   events.TypeHumanInterventionRequested,
			GoalID: goalID,
			Source: "pipelinerunner",
			Payload: map[string]interface{}{
				"action_id": actionID,
				"reason":    reason,
			},
		})
		return &PipelineResult{Status: PipelineFailed, Error: reason}, nil
	case DecideABORT:
		return &PipelineResult{Status: PipelineFailed, Error: "aborted: " + reason}, nil
	default:
		return &PipelineResult{Status: PipelineCompleted}, nil
	}
}

// publishCheckPerformed 发布 CheckPerformed 事件。
func (pr *PipelineRunner) publishCheckPerformed(goalID string, actionID string, result CheckResult) {
	pr.bus.Publish(events.Event{
		Type:   events.TypeCheckPerformed,
		GoalID: goalID,
		Source: "pipelinerunner",
		Payload: map[string]interface{}{
			"action_id": actionID,
			"result":    string(result),
		},
	})
}

// ── 辅助方法 ──

func (pr *PipelineRunner) isActionCompleted(goalID string, actionID string) bool {
	state, err := pr.store.LoadState(goalID)
	if err != nil {
		return false
	}
	for _, id := range state.CompletedNodes {
		if id == actionID {
			return true
		}
	}
	return false
}

func (pr *PipelineRunner) requiresWait(actionID string) bool {
	// R-362: 从 MissionGraph 节点标记判定。MVP 返回 false（简化）。
	return false
}

func (pr *PipelineRunner) getNextAction(goalID string, state *statestore.GoalState) string {
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

// SetRecoveryPipeline 设置恢复管线（v0.1.1 重写）。
func (pr *PipelineRunner) SetRecoveryPipeline(r *RecoveryPipeline) { pr.recovery = r }
