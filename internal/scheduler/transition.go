// Package scheduler 实现 GoalOS Scheduler——纯状态机驱动者。
//
// Transition 是 Kernel 的纯函数核心：
//
//	输入：(当前状态, 事件) → 输出：(新状态, 待发布事件, 是否匹配)
//
// 无副作用。无 I/O。无 LLM 调用。无 Agent 调用。
// Scheduler 是 Action 状态机的唯一拥有者和维护者（R229）。
//
// 设计依据：05 架构文档 §3、R153、R216、R229。
package scheduler

import "github.com/goalos/goalos/pkg/events"

// GoalStatus 是 Goal 的内部状态。
type GoalStatus string

const (
	StatusDraft       GoalStatus = "draft"           // 草稿
	StatusPlanned     GoalStatus = "planned"         // 已规划
	StatusRunning     GoalStatus = "running"         // 执行中
	StatusPaused      GoalStatus = "paused"          // 已暂停
	StatusRecovering  GoalStatus = "recovering"      // 恢复中
	StatusCompleted   GoalStatus = "completed"       // 已完成
	StatusFailed      GoalStatus = "failed_terminal" // 失败（不可恢复）
	StatusRollingBack GoalStatus = "rolling_back"    // 回滚中
)

// ActionStatus 是 Action 级状态（R218）。Scheduler 唯一拥有和维护。
type ActionStatus string

const (
	ActionPending    ActionStatus = "pending"    // 初始状态
	ActionScheduled  ActionStatus = "scheduled"  // Scheduler 已调度
	ActionApproved   ActionStatus = "approved"   // Governance 已批准
	ActionExecuting  ActionStatus = "executing"  // Plugin Runner 正在执行
	ActionVerifying  ActionStatus = "verifying"  // 验证阶段（Verifier 判定中）
	ActionCompleted  ActionStatus = "completed"  // 已完成
	ActionFailed     ActionStatus = "failed"     // 失败
	ActionRecovering ActionStatus = "recovering" // Recovery 进行中
)

// Transition 是纯函数：当前状态 + 事件 → 新状态 + 待发布事件列表。
// 第三个返回值表示是否匹配到转换规则。false = 未知事件，状态不变。
func Transition(current GoalStatus, evtType string) (GoalStatus, []string, bool) {
	type row struct {
		from GoalStatus
		evt  string
		to   GoalStatus
		emit []string
	}
	matrix := []row{
		{StatusDraft, events.TypeMissionGenerated, StatusPlanned, nil},
		{StatusPlanned, events.TypeUserConfirmed, StatusRunning, []string{events.TypeActionScheduled}},
		{StatusPlanned, events.TypeUserRejected, StatusDraft, nil},
		{StatusRunning, events.TypeGoalPauseRequested, StatusPaused, []string{events.TypeGoalPaused}},
		{StatusPaused, events.TypeGoalResumed, StatusRunning, []string{events.TypeGoalResumed}},
		{StatusRunning, events.TypeGoalCompleted, StatusCompleted, nil},
		{StatusRunning, events.TypeGoalRollbackRequested, StatusRollingBack, []string{events.TypeActionRolledBack}},
		{StatusRollingBack, events.TypeActionRolledBack, StatusRunning, nil},
		{StatusRollingBack, events.TypeRecoveryFailed, StatusFailed, []string{events.TypeHumanInterventionRequested}},
		{StatusRunning, events.TypeActionFailed, StatusRecovering, []string{events.TypeActionRetrying}},
		{StatusRecovering, events.TypeActionCompleted, StatusRunning, nil},
		{StatusRecovering, events.TypeRecoveryFailed, StatusFailed, []string{events.TypeHumanInterventionRequested}},
		// Verification Loop transitions (R145-R148)
		{StatusRunning, events.TypeVerificationFailed, StatusRunning, nil},
		{StatusRunning, events.TypeSelfCorrectionExhausted, StatusFailed, []string{events.TypeHumanInterventionRequested}},
		// 异步审批竞态: Paused 状态下拒绝 ActionApproved
		{StatusPaused, events.TypeActionApproved, StatusPaused, []string{events.TypeActionRejected}},
	}

	for _, r := range matrix {
		if r.from == current && r.evt == evtType {
			return r.to, r.emit, true
		}
	}
	return current, nil, false
}

// UserVisible 将内部状态映射为用户可见状态。
func UserVisible(internal GoalStatus) string {
	switch internal {
	case StatusDraft, StatusPlanned, StatusRunning, StatusRecovering:
		return "进行中"
	case StatusPaused, StatusFailed, StatusRollingBack:
		return "需要处理"
	case StatusCompleted:
		return "已完成"
	default:
		return "进行中"
	}
}
