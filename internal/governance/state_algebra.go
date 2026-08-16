// state_algebra.go — 状态代数矩阵四维单一权威定义（R-1092~R-1096/R-1136/R-1343/
// R-1407——任务 3.26）。
//
// 契约（05 §3.2 为权威，D1 修正——四维补全 R-1136）:
//   GoalPhase    Goal 级状态（含 Stopped(user_stopped) 终态——不伪装 Failed，R-1095）
//   ActionPhase  Action 级状态（终态=Completed/Failed/Timeout/Cancelled——R-1405 补 Cancelled）
//   PipelinePhase Pipeline 级状态（四值循环性质——无终态，R-1136）
//   ApprovalPhase Approval 级状态（终态=Approved/Rejected——TimedOut 终态删除，
//     超时=Rejected(approval_timeout) 单一结局，R-1343）
// 非法迁移=拒绝（R-1407 交叉约束规则集）+StateMachineViolation 发布（R-1362）。
package governance

import (
	"fmt"

	"github.com/goalos/goalos/pkg/events"
)

// GoalPhase — Goal 级状态（四维矩阵第一维）。
type GoalPhase string

const (
	GoalDraft     GoalPhase = "draft"
	GoalPlanning  GoalPhase = "planning"
	GoalRunning   GoalPhase = "running"
	GoalPaused    GoalPhase = "paused"
	GoalStopped   GoalPhase = "stopped"    // user_stopped——用户主动停止，终态（R-1095 不伪装 Failed）
	GoalCompleted GoalPhase = "completed"  // 终态
	GoalFailed    GoalPhase = "failed"     // 终态
)

// Terminal — Goal 终态判定（R-1095：Stopped 与 Failed 各自独立终态）。
func (s GoalPhase) Terminal() bool {
	return s == GoalStopped || s == GoalCompleted || s == GoalFailed
}

// ActionPhase — Action 级状态（四维矩阵第二维；R-1136 七值）。
type ActionPhase string

const (
	ActionPending    ActionPhase = "pending"
	ActionExecuting  ActionPhase = "executing"
	ActionVerifying  ActionPhase = "verifying"
	ActionCompleted  ActionPhase = "completed"  // 终态
	ActionFailed     ActionPhase = "failed"     // 终态
	ActionTimeout    ActionPhase = "timeout"    // 终态
	ActionCancelled  ActionPhase = "cancelled"  // 终态（R-1405 补 Cancelled）
)

// Terminal — Action 终态判定（终态=Completed/Failed/Timeout/Cancelled——R-1405）。
func (s ActionPhase) Terminal() bool {
	switch s {
	case ActionCompleted, ActionFailed, ActionTimeout, ActionCancelled:
		return true
	}
	return false
}

// PipelinePhase — Pipeline 级状态（四维矩阵第三维；R-1136 四值，无终态——循环性质）。
type PipelinePhase string

const (
	PipelineCheck  PipelinePhase = "check"
	PipelineExec   PipelinePhase = "exec"
	PipelineWait   PipelinePhase = "wait"
	PipelineDecide PipelinePhase = "decide"
)

// ApprovalPhase — Approval 级状态（四维矩阵第四维；R-1343 TimedOut 终态删除）。
type ApprovalPhase string

const (
	ApprovalPending  ApprovalPhase = "pending"
	ApprovalApproved ApprovalPhase = "approved"  // 终态
	ApprovalRejected ApprovalPhase = "rejected"  // 终态（超时=Rejected(approval_timeout) 单一结局）
)

// Terminal — Approval 终态判定（终态=Approved/Rejected——R-1343）。
func (s ApprovalPhase) Terminal() bool {
	return s == ApprovalApproved || s == ApprovalRejected
}

// StateMatrix — 四维状态代数矩阵（GoalPhase×ActionPhase×PipelinePhase×ApprovalPhase）
// 的单一权威定义（R-1092~R-1096/R-1136——D1 四维补全）。
type StateMatrix struct {
	Goal     GoalPhase     // Goal 级
	Action   ActionPhase   // Action 级
	Pipeline PipelinePhase // Pipeline 级
	Approval ApprovalPhase // Approval 级
}

// ValidateTransition — 迁移合法性判定（R-1407 交叉约束规则集）。
// 非法迁移返回描述性 error（字段名+约束名）——不静默降级。
// 交叉约束（R-1407）:
//   1. Goal 终态后不得进入非终态 Goal（Stopped/Completed/Failed 无后继——R-1362）
//   2. Goal 终态下 Pipeline 必须为 check/decide（终态后无 Exec/Wait——执行已结束）
//   3. Approval=Rejected 下 Action 不得宣称 Completed（拒绝后无完成路径）
//   4. Approval=Approved 下 Action 不得停留在 Pending（批准后必须进入执行）
func (m *StateMatrix) ValidateTransition(next StateMatrix) error {
	if m.Goal.Terminal() && !next.Goal.Terminal() {
		return fmt.Errorf("state_algebra: goal=%s 为终态，非法迁移至 %s（终态无后继——R-1362/R-1407）", m.Goal, next.Goal)
	}
	if next.Goal.Terminal() {
		switch next.Pipeline {
		case PipelineCheck, PipelineDecide:
			// 终态下仅允许收尾原语
		default:
			return fmt.Errorf("state_algebra: goal=%s 终态下 pipeline=%s 非法（终态后无 Exec/Wait——R-1407）", next.Goal, next.Pipeline)
		}
	}
	if m.Approval == ApprovalRejected && next.Action == ActionCompleted {
		return fmt.Errorf("state_algebra: approval=rejected 下 action=completed 非法（拒绝后无完成路径——R-1407）")
	}
	if m.Approval == ApprovalApproved && next.Action == ActionPending {
		return fmt.Errorf("state_algebra: approval=approved 下 action=pending 非法（批准后必须进入执行——R-1407）")
	}
	return nil
}

// RejectIllegalTransition — Engine 级非法迁移闸门（R-1362）。
// 非法迁移=发布 StateMachineViolation + 返回描述性 error——拒绝执行（不静默放行）。
// 合法迁移返回 nil 且不发布事件。
func (e *Engine) RejectIllegalTransition(goalID string, cur, next StateMatrix) error {
	if err := cur.ValidateTransition(next); err != nil {
		e.bus.Publish(events.Event{
			Type:    events.TypeStateMachineViolation,
			GoalID:  goalID,
			Source:  "governance",
			Payload: map[string]interface{}{
				"goal_id":              goalID,
				"current_state":        fmt.Sprintf("%s/%s/%s/%s", cur.Goal, cur.Action, cur.Pipeline, cur.Approval),
				"attempted_transition": fmt.Sprintf("%s/%s/%s/%s", next.Goal, next.Action, next.Pipeline, next.Approval),
				"violation":            err.Error(),
			},
		})
		return err
	}
	return nil
}
