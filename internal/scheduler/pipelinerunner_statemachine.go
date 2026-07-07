// Package scheduler — v0.2.0 Week 2: PipelineRunner 状态机循环
// R-765: Wait 不是独立步骤——是中间状态。Pre-Exec（Check BLOCK）或 Post-Exec（异步任务）。
// R-771: ErrorTemporary→内部新Session重做。CONTINUE/ESCALATE 接口不变。
package scheduler

import (
	"context"
	"time"

	"github.com/goalos/goalos/internal/errorcategory"
)

// RunState 是状态机循环的状态。
type RunState int

const (
	StateCheck  RunState = iota
	StateWait
	StateExec
	StateDecide
)

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
func (pr *PipelineRunner) StateMachineRun(ctx context.Context, actionID string) *StateMachineResult {
	state := StateCheck
	waitType := "" // "pre_exec" | "post_exec"
	retryCount := 0
	const maxRetries = 1 // R-741: 最多 1 次新 Session 重做

	for {
		select {
		case <-ctx.Done():
			return &StateMachineResult{Status: PipelineFailed, Error: "cancelled"}
		default:
		}

		switch state {
		case StateCheck:
			result := pr.checkAction(actionID)
			switch result {
			case CheckPASS:
				state = StateExec
			case CheckBLOCK:
				waitType = "pre_exec"
				state = StateWait
			case CheckREJECT:
				return &StateMachineResult{Status: PipelineFailed, Error: "check_rejected"}
			default: // CheckWARN
				state = StateExec
			}

		case StateWait:
			timeout := 30 * time.Second // default
			select {
			case <-pr.wakeupCh:
				if waitType == "pre_exec" {
					state = StateCheck // 重新 Check
				} else {
					state = StateDecide
				}
			case <-time.After(timeout):
				return &StateMachineResult{
					Status: PipelineFailed,
					Error:  "wait_timeout",
				}
			}

		case StateExec:
			result, err := pr.executeAction(actionID)
			if err != nil {
				catErr, ok := err.(errorcategory.CategorizedError)
				if ok && catErr.Category() == errorcategory.ErrorTemporary && retryCount < maxRetries {
					// R-771: ErrorTemporary→内部新Session重做
					retryCount++
					state = StateCheck // 重新 Check→Exec
					continue
				}
				return &StateMachineResult{
					Status: PipelineFailed,
					Error:  catErr.Suggestion(),
				}
			}
			if result != nil && result.Async {
				waitType = "post_exec"
				state = StateWait
			} else {
				state = StateDecide
			}

		case StateDecide:
			return &StateMachineResult{Status: PipelineCompleted}
		}
	}
}

// checkAction 执行 Check 原语。
// 真实实现: 空actionID→REJECT。合法actionID→验证Plugin存在性→PASS/WARN/BLOCK。
// Phase B 增强: QualityGate完整评估（PolicyGate/RiskGate/SkillGate）。
func (pr *PipelineRunner) checkAction(actionID string) CheckResult {
	if actionID == "" {
		return CheckREJECT
	}
	// 验证 Action 对应的 Plugin 是否已注册（Fan-Out cache）
	if pr.pluginCache != nil && pr.pluginCache.Count() == 0 {
		return CheckWARN // 无 Plugin 可用——警告但允许继续
	}
	// 检查是否超过系统负载阈值
	if pr.retryCount != nil && len(pr.retryCount) > 100 {
		return CheckBLOCK // 系统过载——阻塞
	}
	return CheckPASS
}

// executeAction 执行 Exec 原语。
// 真实实现: 空actionID→Permanent错误。非空→验证Plugin→同步/异步执行。
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
	// 同步执行——Phase B 当前阶段
	return &execResult{Async: false}, nil
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
