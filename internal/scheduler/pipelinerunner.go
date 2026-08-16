// Package scheduler — PipelineRunner v0.2.2。
// Action 级执行引擎。对每个 Action 执行 Check→Exec→Decide 三原语管线。
// Wait 为 PipelineRunner 中间状态（非 Primitive）。Decide 路径收敛为 CONTINUE/ESCALATE。
//
// 设计依据：05 架构文档 §3.1、会议 #79 E1、会议 #107。
package scheduler

import (
	"context"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/goalos/goalos/internal/errorcategory"
	"github.com/goalos/goalos/internal/eventbus"
	"github.com/goalos/goalos/internal/governance"
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

// S'-12 改名映射表（会议 #200/R-1255，任务 3.26 完成态）:
//   - 原 scheduler.RunState → PipelineState 枚举（四值顺序按 D1: Check/Exec/Wait/Decide）
//   - 原 scheduler.ResumePrimitive → PipelinePrimitive 四值（补 exec——语义=重执行
//     该动作节点，R-1112）
//   - 原 scheduler.PipelineState（struct）→ 并入 statestore.PipelineSnapshot
//     （字段重叠合并——执行位置快照的唯一类型）
// Go 类型层唯一类型与别名关系：PipelineState 与 PipelinePrimitive 均为
// governance.PipelinePhase 四值枚举的别名（状态代数矩阵 Pipeline 维单一权威，
// R-1136/D1——无终态，循环性质）。
type (
	PipelineState    = governance.PipelinePhase // 状态机循环状态四值（check/exec/wait/decide）
	PipelinePrimitive = governance.PipelinePhase // 恢复原语四值（补 exec——R-1112 重执行）
)

const (
	StateCheck  = governance.PipelineCheck
	StateExec   = governance.PipelineExec
	StateWait   = governance.PipelineWait
	StateDecide = governance.PipelineDecide
)

// PipelinePrimitive 四值恢复原语（S'-12——原 ResumePrimitive 三值并入四值枚举）。
const (
	ResumeFromCheck  = governance.PipelineCheck
	ResumeFromExec   = governance.PipelineExec  // 补 exec——语义=重执行该动作节点（R-1112）
	ResumeFromDecide = governance.PipelineDecide
	ResumeFromWait   = governance.PipelineWait
)

// PipelineResult 是 PipelineRunner.Run() 的返回值。
// PipelineState 字段类型=S'-12 改名后唯一类型 statestore.PipelineSnapshot。
type PipelineResult struct {
	Status        PipelineStatus
	Error         string
	WaitReason    string
	PipelineState *statestore.PipelineSnapshot
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
	DecideESCALATE DecidePath = "ESCALATE"
	// H3: RETRY/REPLAN/ABORT 已废除（会议 #107）
)

// PipelineRunner 是 Action 级执行引擎（v0.1.0 重写）。
type PipelineRunner struct {
	bus      *eventbus.EventBus
	store    *statestore.Store
	state    *statestore.PipelineSnapshot
	multiLLM     *MultiLLMVerifier
	retryCount   map[string]int
	wakeupCh       chan struct{}       // R-765: Wait 唤醒通道
	pluginCache    *PluginCache        // R-737: Fan-Out 本地 Plugin cache
	actionCode     map[string]string   // CR-K3: actionID→code for MultiLLM check
	workspaceDir   string              // R-837: Goal workspace output dir
	currentGoalID  string              // R-840: 当前 Goal ID
	// approvalTimeout 是 Wait 状态的最长等待时长（R-1384/R-1343: 单一计时权威）。
	// 来源：policy.approval_timeout 配置（daemon.yaml），与 Governance 审批超时同源。
	// Daemon 构造时经 SetApprovalTimeout 注入；缺省 300s。
	approvalTimeout time.Duration
}

// NewPipelineRunner 创建 PipelineRunner。
// R-836: 订阅 ActionCompleted——提取产出代码注入 actionCode 供 MultiLLM Check 验证。
func NewPipelineRunner(bus *eventbus.EventBus, store *statestore.Store) *PipelineRunner {
	pr := &PipelineRunner{
		bus:             bus,
		store:           store,
		approvalTimeout: 300 * time.Second, // R-1384: 缺省 300s（policy.approval_timeout）
		retryCount:      make(map[string]int),
		wakeupCh:        make(chan struct{}, 10),
		pluginCache:     NewPluginCache(),
		actionCode:      make(map[string]string),
	}
	// R-836: 订阅 ActionCompleted——提取代码产出供下一轮 MultiLLM 验证
	bus.Subscribe(events.TypeActionCompleted, func(evt events.Event) error {
		if actionID, ok := evt.Payload["action_id"].(string); ok && actionID != "" {
			if output, ok := evt.Payload["output"].(string); ok && output != "" {
				pr.actionCode[actionID] = output
			}
		}
		return nil
	})
	return pr
}

// Run 执行 MissionGraph 的 Action 原语管线。
// v0.2.0 W1: 统一为状态机版本 StateMachineRun。删除旧的线性 executePrimitivePipeline。
// 返回 PipelineResult 保持与 GoalRunner 的接口兼容。
func (pr *PipelineRunner) Run(goalID string, state *statestore.GoalState) (*PipelineResult, error) {
	pr.currentGoalID = goalID // R-840: 供 publishVerdict 使用
	if state.PipelineState != nil {
		// S'-12: 执行位置快照唯一类型=statestore.PipelineSnapshot——直接复制（无跨包转换）。
		pr.state = state.PipelineState
		log.Printf("[PipelineRunner] goal=%s resumed from %s primitive at node %s",
			goalID, pr.state.ResumePrimitive, pr.state.ResumePoint)
	} else {
		pr.state = &statestore.PipelineSnapshot{}
	}

	// 恢复路径
	if PipelinePrimitive(pr.state.ResumePrimitive) == ResumeFromWait {
		return pr.wait(goalID, pr.state.WaitReason)
	}
	if PipelinePrimitive(pr.state.ResumePrimitive) == ResumeFromDecide {
		return pr.decide(goalID, "", nil)
	}

	// 获取下一个待执行 Action
	currentAction := pr.getNextAction(goalID, state)
	if currentAction == "" {
		return &PipelineResult{Status: PipelineCompleted}, nil
	}

	// R-837: 设置 workspace 目录供 MultiLLM 读文件
	pr.workspaceDir = os.Getenv("HOME") + "/Goals/" + goalID + "/output"
	// v0.2.0 W1: 使用状态机循环（Check→Wait→Exec→Decide）
	logProgress(goalID, "Check", "evaluating action: "+currentAction)
	ctx := context.Background()
	result := pr.StateMachineRun(ctx, goalID, currentAction)

	switch result.Status {
	case PipelineCompleted:
		return &PipelineResult{Status: PipelineCompleted}, nil
	case PipelineFailed:
		return &PipelineResult{Status: PipelineFailed, Error: result.Error}, nil
	case PipelineWaiting:
		// P10+P11: WaitReason 和 ResumePrimitive 从 StateMachineResult 提取
		waitReason := "approval"
		resumeFrom := ResumeFromCheck
		if len(result.PendingWaits) > 0 {
			if result.PendingWaits[0].Type == "post_exec" {
				resumeFrom = ResumeFromDecide // post-exec wait→从 Decide 恢复
			}
			waitReason = result.PendingWaits[0].Type
		}
		return &PipelineResult{
			Status:     PipelineWaiting,
			WaitReason: waitReason,
			PipelineState: &statestore.PipelineSnapshot{
				ResumePrimitive:  string(resumeFrom),
				PendingActionIDs: []string{currentAction},
			},
		}, nil
	default:
		return nil, fmt.Errorf("pipelinerunner: unknown state from StateMachineRun: %v", result.Status)
	}}

// check 评估 Action 的准入条件。v0.1.1 重写：集成 MultiLLMVerifier。
// check 评估 Action 的准入条件（Pre-Exec Gate）。
// R-840: MultiLLM 代码审查已移到 decide()——Check 只做准入检查。
// v0.3.0 fix (C1): 检查 Action 是否已经过 Governance 审批。
func (pr *PipelineRunner) check(actionID string, code ...string) CheckResult {
	// 基础校验：actionID 非空
	if actionID == "" {
		return CheckREJECT
	}
	// 检查 Governance 审批状态——从 store 中确认 ActionApproved 已发布
	if pr.store != nil {
		state, err := pr.store.LoadState(pr.currentGoalID)
		if err == nil && state != nil {
			// 检查 ApprovalPending——若挂起则 BLOCK，等待审批
			if state.ApprovalPending {
				log.Printf("[PipelineRunner] check: action=%s approval pending — BLOCK", actionID)
				return CheckBLOCK
			}
			// 检查是否已完成（幂等保护）
			for _, id := range state.CompletedNodes {
				if id == actionID {
					return CheckPASS
				}
			}
		}
	}
	// R-839: Scheduler/Governance 已通过 ActionApproved → Check PASS
	return CheckPASS
}

// exec 标记 Action 进入执行阶段，发布 ActionStarted 事件。
// v0.3.0 fix (C2): 发布 ActionStarted——PluginRunner 已在 ActionApproved 中异步执行。
func (pr *PipelineRunner) exec(goalID string, actionID string) error {
	log.Printf("[PipelineRunner] goal=%s action=%s: executing", goalID, actionID)
	if pr.bus != nil {
		pr.bus.Publish(events.Event{
			Type:   events.TypeActionStarted,
			GoalID: goalID,
			Source: "pipelinerunner",
			Payload: map[string]interface{}{
				"action_id": actionID,
			},
		})
	}
	return nil
}

// logProgress 输出用户可见的进度日志（CR-J1）。
func logProgress(goalID, stage, detail string) {
	log.Printf("[PipelineRunner] goal=%s | %s | %s", goalID, stage, detail)
}

// wait 进入等待状态。保存 PipelineState 并返回 WAITING。
func (pr *PipelineRunner) wait(goalID string, reason string) (*PipelineResult, error) {
	pr.state.ResumePrimitive = string(ResumeFromDecide)
	pr.state.WaitReason = reason
	// R-1384/R-1343: 计时单一权威——读 policy.approval_timeout（SetApprovalTimeout 注入），
	// 不再硬编码 5min。GoalRunner.waitForWakeup 依据 TimeoutAt 计算实际等待时长。
	pr.state.TimeoutAt = time.Now().Add(pr.approvalTimeout).Format(time.RFC3339)

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

// decide 决策下一步（R-840: MultiLLM 同步验证集成）。
func (pr *PipelineRunner) decide(goalID string, actionID string, execErr error) (*PipelineResult, error) {
	if execErr != nil {
		path := ClassifyError(execErr)
		return pr.decidePath(goalID, actionID, path, execErr.Error())
	}
	// R-840: Exec 成功→MultiLLM 代码审查（同步，Decide 阶段）
	if pr.multiLLM != nil {
		code := readWorkspaceCode(pr.workspaceDir)
		if code != "" {
			log.Printf("[PipelineRunner] MultiLLM sync verify: action=%s code_len=%d", actionID, len(code))
			verdict, err := pr.multiLLM.Verify(code, actionID)
			if err == nil {
				pr.publishVerdict(actionID, verdict)
				switch verdict.Result {
				case "FAIL":
					return pr.decidePath(goalID, actionID, DecideESCALATE, "multi_llm_fail: code review failed") // REPLAN
				case "WARN":
					return pr.decidePath(goalID, actionID, DecideCONTINUE, "multi_llm_warn")
				default: // PASS
					return pr.decidePath(goalID, actionID, DecideCONTINUE, "")
				}
			}
			log.Printf("[PipelineRunner] MultiLLM verify error: %v", err)
		}
	}
	return pr.decidePath(goalID, actionID, DecideCONTINUE, "")
}

// publishVerdict 发布 MultiLLM 裁决结果（R-840）。
func (pr *PipelineRunner) publishVerdict(actionID string, verdict *Verdict) {
	if pr.bus == nil {
		return
	}
	// R-840: 发布完整裁决——含各 Provider 个体意见
	votes := make([]map[string]interface{}, len(verdict.Votes))
	for i, v := range verdict.Votes {
		votes[i] = map[string]interface{}{
			"provider":  v.Provider,
			"model":     v.Model,
			"vote":      v.Vote,
			"reasoning": v.Reasoning,
		}
	}
	pr.bus.Publish(events.Event{
		Type:   "MultiLLMVerificationCompleted",
		Source: "pipelinerunner",
		GoalID: pr.currentGoalID,
		Payload: map[string]interface{}{
			"action_id": actionID,
			"verdict":   verdict.Result,
			"score":     verdict.WeightedScore,
			"consensus": verdict.Consensus,
			"votes":     votes,
		},
	})
}

// recoveryActionToDecidePath 将 RecoveryPath.Action 映射为 DecidePath。
// H1: RETRY/AUTO_FIX/SWITCH_TOOL 已废除（会议 #107）。收敛为 CONTINUE/ESCALATE。
func recoveryActionToDecidePath(action string) DecidePath {
	if action == "ESCALATE" {
		return DecideESCALATE
	}
	return DecideCONTINUE
}

// decidePath 发布 DecidePathSelected 事件并返回对应 PipelineResult。
func (pr *PipelineRunner) decidePath(goalID string, actionID string, path DecidePath, reason string) (*PipelineResult, error) {
	// P16: nil bus guard
	if pr.bus != nil {
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
	}

	switch path {
	case DecideCONTINUE:
		return &PipelineResult{Status: PipelineCompleted}, nil
	case DecideESCALATE:
		if pr.bus != nil {
			pr.bus.Publish(events.Event{
				Type:   events.TypeHumanInterventionRequested,
				GoalID: goalID,
				Source: "pipelinerunner",
				Payload: map[string]interface{}{
					"action_id": actionID,
					"reason":    reason,
				},
			})
		}
		return &PipelineResult{Status: PipelineFailed, Error: reason}, nil
	default:
		return nil, fmt.Errorf("pipelinerunner: unknown decide path: %s", path)
	}
}

// publishCheckPerformed 发布 CheckPerformed 事件。
func (pr *PipelineRunner) publishCheckPerformed(goalID string, actionID string, result CheckResult) {
	if pr.bus == nil {
		return // R-815: nil bus tolerated for test environments
	}
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
	if pr.store == nil {
		return false
	}
	state, err := pr.store.LoadState(goalID)
	if err != nil {
		log.Printf("[PipelineRunner] isActionCompleted: LoadState error for %s: %v — skipping to avoid duplicate", goalID, err)
		return true // CR-B2: 出错时保守返回 true，避免重复执行
	}
	for _, id := range state.CompletedNodes {
		if id == actionID {
			return true
		}
	}
	return false
}

func (pr *PipelineRunner) requiresWait(goalID string, actionID string) bool {
	// v0.2.0 W1 fix: nil guard for store
	if pr.store == nil {
		return false
	}
	// v0.2.0 audit fix: 从 GoalState 检查是否有待审批 Action
	state, err := pr.store.LoadState(goalID)
	if err != nil {
		return false
	}
	if state.ApprovalPending {
		return true
	}
	// 检查 PipelineState 中是否有等待中的 Action
	if state.PipelineState != nil {
		for _, id := range state.PipelineState.PendingActionIDs {
			if id == actionID {
				return true
			}
		}
	}
	return false
}

func (pr *PipelineRunner) getNextAction(goalID string, state *statestore.GoalState) string {
	// P22: nil guard
	if state == nil {
		return ""
	}
	// R-839: 多节点支持——遍历 NodeIDs
	if len(state.NodeIDs) > 0 {
		for _, nid := range state.NodeIDs {
			if !containsStr(state.CompletedNodes, nid) {
				return nid
			}
		}
		log.Printf("[PipelineRunner] goal=%s: all %d nodes completed", goalID, len(state.NodeIDs))
		return ""
	}
	// 向后兼容：单 NodeID
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

// readWorkspaceCode 读取 workspace 产出目录中的代码文件（R-837 MultiLLM 验证）。
func readWorkspaceCode(workspaceDir string) string {
	entries, err := os.ReadDir(workspaceDir)
	if err != nil {
		return ""
	}
	var buf strings.Builder
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".js") && !strings.HasSuffix(e.Name(), ".html") && !strings.HasSuffix(e.Name(), ".css") {
			continue
		}
		data, err := os.ReadFile(filepath.Join(workspaceDir, e.Name()))
		if err != nil {
			continue
		}
		buf.Write(data)
		buf.WriteByte('\n')
	}
	return buf.String()
}

// SetMultiLLM 设置多模型验证器（v0.1.1）。
func (pr *PipelineRunner) SetMultiLLM(v *MultiLLMVerifier) { pr.multiLLM = v }

// SetApprovalTimeout 设置 Wait 最长等待时长（R-1384/R-1343: 单一计时权威）。
// 由 Daemon 在构造 PipelineRunner 时注入 cfg.Policy.ApprovalTimeout；
// 未注入时缺省 300s。与 Governance 审批超时（gov.SetApprovalTimeout）同源。
func (pr *PipelineRunner) SetApprovalTimeout(d time.Duration) {
	if d > 0 {
		pr.approvalTimeout = d
	}
}

// ─── Week 0 框架基础设施 F3+F5: CategorizedError 路由 + Validatable 集成 ───

// ClassifyError 读取 CategorizedError.Category()→返回 DecidePath（R-771）。
// ErrorTemporary→内部重试（新 Session），其他→ESCALATE。
// 非 CategorizedError→默认 ErrorFatal。
func ClassifyError(err error) DecidePath {
	// P23: nil error → CONTINUE（无错误，无需决策）
	if err == nil {
		return DecideCONTINUE
	}
	cat := errorcategory.CategoryOf(err)
	switch cat {
	case errorcategory.ErrorTemporary:
		return DecideCONTINUE // R-771: 内部触发新 Session 重做
	case errorcategory.ErrorPermanent, errorcategory.ErrorSecurity:
		return DecideESCALATE
	case errorcategory.ErrorFatal:
		return DecideESCALATE // GoalRunner 收到后→GoalFailed
	default:
		return DecideCONTINUE // H3: 未知错误默认继续（会议 #107 收敛为 CONTINUE/ESCALATE）
	}
}

// ValidatePayload 调用 payload 的 Validate()（若实现 Validatable）（R-770）。
// EventBus.Publish() 内部调用——当前作为 PipelineRunner 辅助函数。
// 校验失败→返回 error，调用方发布 InvariantViolated + ActionFailed。
func ValidatePayload(payload interface{ Validate() error }) error {
	return payload.Validate()
}
