// Package scheduler — v0.2.0 Week 2: PipelineRunner 状态机循环
// R-765: Wait 不是独立步骤——是中间状态。Pre-Exec（Check BLOCK）或 Post-Exec（异步任务）。
// R-771: ErrorTemporary→内部新Session重做。CONTINUE/ESCALATE 接口不变。
package scheduler

import (
	"context"
	"log"
	"time"

	"github.com/goalos/goalos/internal/errorcategory"
)

// S'-12 改名映射表: 原 RunState → PipelineState 枚举（四值顺序按 D1:
// Check/Exec/Wait/Decide）。唯一类型与别名关系见 pipelinerunner.go——
// PipelineState = governance.PipelinePhase 别名，State* 常量为同包别名。

// StateMachineResult 是状态机循环的返回。
type StateMachineResult struct {
	Status       PipelineStatus
	Error        string
	PendingWaits []WaitCondition
}

// WaitCondition 单个等待条件（R-764）。
type WaitCondition struct {
	Type     string    // "approval" | "dependency" | "resource"
	TargetID string
	Timeout  time.Time
}

// StateMachineRun 执行 PipelineRunner 状态机循环（R-765 + R-771）。
// 替代旧的线性 Check→Exec→Wait→Decide 管线。
// v0.3.0 fix: 设置 currentGoalID 以支持 check() 中的 Governance 审批状态查询。
func (pr *PipelineRunner) StateMachineRun(ctx context.Context, goalID string, actionID string) *StateMachineResult {
	pr.currentGoalID = goalID
	state := StateCheck
	const maxRetries = 1 // R-741: 最多 1 次新 Session 重做
	// S4: retryCount 使用 pr.retryCount[actionID]（跨 StateMachineRun 调用持久）

	for {
		select {
		case <-ctx.Done():
			return &StateMachineResult{Status: PipelineFailed, Error: "cancelled"}
		default:
		}

		switch state {
		case StateCheck:
			result := pr.checkAction(actionID)
			pr.publishCheckPerformed(goalID, actionID, result) // H10: 发布 CheckPerformed
			switch result {
			case CheckPASS:
				state = StateExec
			case CheckBLOCK:
				state = StateWait
			case CheckREJECT:
				return &StateMachineResult{Status: PipelineFailed, Error: "check_rejected"}
			default: // CheckWARN
				state = StateExec
			}

		case StateWait:
			timeout := 30 * time.Second // default
			select {
			case <-ctx.Done():
				return &StateMachineResult{Status: PipelineFailed, Error: "cancelled"}
			case <-pr.wakeupCh:
				state = StateCheck // CheckBLOCK解除→重新 Check
			case <-time.After(timeout):
				return &StateMachineResult{
					Status: PipelineFailed,
					Error:  "wait_timeout",
				}
			}

		case StateExec:
			// v0.2.0 W1: Check requiresWait before exec
			if pr.requiresWait(goalID, actionID) {
				return &StateMachineResult{
					Status:       PipelineWaiting,
					PendingWaits: []WaitCondition{{Type: "approval", TargetID: actionID}},
				}
			}
			result, err := pr.executeAction(actionID)
			if err != nil {
				catErr, ok := err.(errorcategory.CategorizedError)
				if ok && catErr.Category() == errorcategory.ErrorTemporary && pr.retryCount[actionID] < maxRetries {
					// R-771: ErrorTemporary→内部新Session重做
					pr.retryCount[actionID]++
					state = StateCheck // 重新 Check→Exec
					continue
				}
				// v0.2.0 W1 fix: 当 err 不实现 CategorizedError 或 ok=false 时，
				// catErr 是 nil——不能调用 catErr.Suggestion()。
				errMsg := err.Error()
				if ok {
					errMsg = catErr.Suggestion()
				}
				return &StateMachineResult{
					Status: PipelineFailed,
					Error:  errMsg,
				}
			}
			// v0.2.0 W2 fix: Async=true → 返回 PipelineWaiting，由 GoalRunner 接管外部事件唤醒。
			// 不再在 StateMachineRun 内部阻塞 Wait。
			if result != nil && result.Async {
				return &StateMachineResult{
					Status:       PipelineWaiting,
					PendingWaits: []WaitCondition{{Type: "post_exec", TargetID: actionID}},
				}
			}
			state = StateDecide

		case StateDecide:
			// R-840: 调用 decide() —— MultiLLM 同步验证
			result, err := pr.decide(goalID, actionID, nil)
			if err != nil {
				return &StateMachineResult{Status: PipelineFailed, Error: err.Error()}
			}
			return &StateMachineResult{Status: result.Status, Error: result.Error}
		}
	}
}

// checkAction 执行 Check 原语。
// CR-K3: 接受可选 code 参数——传入 MultiLLM 验证器。
func (pr *PipelineRunner) checkAction(actionID string, code ...string) CheckResult {
	if actionID == "" {
		return CheckREJECT
	}
	// CR-K3 + R-836: 从 actionCode 获取代码 → MultiLLM 语义验证
	if c, ok := pr.actionCode[actionID]; ok && c != "" {
		log.Printf("[PipelineRunner] checkAction: MultiLLM code found for %s (%d chars)", actionID, len(c))
		return pr.check(actionID, c)
	}
	// R-837: 文件系统 fallback——读 workspace 产出文件作为 MultiLLM 验证代码
	if c := readWorkspaceCode(pr.workspaceDir); c != "" {
		log.Printf("[PipelineRunner] checkAction: workspace code loaded for %s (%d chars)", actionID, len(c))
		return pr.check(actionID, c)
	}
	// fallthrough to basic checks + governance review
	// 验证 Action 对应的 Plugin 是否已注册（Fan-Out cache）
	if pr.pluginCache != nil && pr.pluginCache.Count() == 0 {
		return CheckWARN
	}
	if pr.retryCount != nil && len(pr.retryCount) > 100 {
		return CheckBLOCK
	}
	// v0.3.0 fix: 最终通过 governance check() 验证审批状态
	return pr.check(actionID)
}

// executeAction 执行 Exec 原语。
// v0.3.0 fix (C3): Scheduler→Governance→PluginRunner 异步执行链。
// PipelineRunner 确认 ActionApproved 已发布后返回 Async=true，
// 让 StateMachineRun 进入 post_exec Wait 等待 ActionCompleted。
func (pr *PipelineRunner) executeAction(actionID string) (*execResult, error) {
	if actionID == "" {
		return nil, &actionError{
			msg:        "actionID is required",
			suggestion: "操作标识无效，请联系系统管理员",
			category:   errorcategory.ErrorPermanent,
		}
	}
	// 查找 Plugin——使用 Fan-Out 本地 cache
	if pr.pluginCache != nil {
		if _, ok := pr.pluginCache.Lookup(actionID); !ok && pr.pluginCache.Count() > 0 {
			return nil, &actionError{
				msg:        "plugin not found for action: " + actionID,
				suggestion: "未找到对应的执行插件，请检查插件是否已安装",
				category:   errorcategory.ErrorPermanent,
			}
		}
	}
	// v0.3.0: PluginRunner 在 ActionApproved 事件中异步执行。
	// 返回 Async=true → StateMachineRun 进入 post_exec Wait，
	// GoalRunner 订阅 ActionCompleted 唤醒后继续 Decide。
	log.Printf("[PipelineRunner] executeAction: action=%s dispatched async — waiting for PluginRunner", actionID)
	return &execResult{Async: true}, nil
}

// actionError 实现 CategorizedError 接口——用于 executeAction 的错误路由。
type actionError struct {
	msg        string // 技术错误信息——Error() 返回（英文）
	suggestion string // 用户可见建议——Suggestion() 返回（中文）
	category   errorcategory.ErrorCategory
}

func (e *actionError) Error() string                         { return e.msg }
func (e *actionError) Category() errorcategory.ErrorCategory { return e.category }
func (e *actionError) Suggestion() string                    { return e.suggestion }

type execResult struct {
	Async bool
}
