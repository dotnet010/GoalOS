// Package scheduler 实现 GoalOS Scheduler——纯状态机驱动者。
//
// Transition 是 Kernel 的纯函数核心：
//   输入：(当前状态, 事件) → 输出：(新状态, 待发布事件, 是否匹配)
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
	StatusDraft       GoalStatus = "draft"          // 草稿——用户已创建 Goal，尚未规划
	StatusPlanned     GoalStatus = "planned"        // 已规划——MissionGraph 已生成
	StatusRunning     GoalStatus = "running"        // 执行中——Action 正在执行
	StatusPaused      GoalStatus = "paused"         // 已暂停——用户或系统暂停
	StatusRecovering  GoalStatus = "recovering"     // 恢复中——Recovery 策略执行中
	StatusCompleted   GoalStatus = "completed"      // 已完成——所有 Action 完成
	StatusFailed      GoalStatus = "failed_terminal" // 失败（不可恢复）——需人工介入
	StatusRollingBack GoalStatus = "rolling_back"   // 回滚中——回滚操作进行中
)

// Transition 是纯函数：当前状态 + 事件 → 新状态 + 待发布事件列表。
// 这是 Kernel 的核心状态转换矩阵。
//
// 第三个返回值表示是否匹配到转换规则。false = 未知事件，状态不变。
func Transition(current GoalStatus, evtType string) (GoalStatus, []string, bool) {
	// Goal 状态转换矩阵。05 架构 §3。
	type row struct {
		from GoalStatus
		evt  string
		to   GoalStatus
		emit []string // 转换后应发布的事件
	}
	matrix := []row{
		// Draft → Planned：MissionGraph 生成完成
		{StatusDraft, events.TypeMissionGenerated, StatusPlanned, nil},
		// Planned → Running：用户确认
		{StatusPlanned, events.TypeUserConfirmed, StatusRunning, []string{events.TypeActionScheduled}},
		// Planned → Draft：用户拒绝
		{StatusPlanned, events.TypeUserRejected, StatusDraft, nil},
		// Running → Paused：用户或系统暂停
		{StatusRunning, events.TypeGoalPauseRequested, StatusPaused, []string{events.TypeGoalPaused}},
		// Paused → Running：用户恢复
		{StatusPaused, events.TypeGoalResumed, StatusRunning, []string{events.TypeGoalResumed}},
		// Running → Completed：全部 Action 完成
		{StatusRunning, events.TypeGoalCompleted, StatusCompleted, nil},
		// Running → RollingBack：用户请求回滚
		{StatusRunning, events.TypeGoalRollbackRequested, StatusRollingBack, []string{events.TypeActionRolledBack}},
		// RollingBack → Running：回滚成功
		{StatusRollingBack, events.TypeActionRolledBack, StatusRunning, nil},
		// RollingBack → Failed：回滚失败
		{StatusRollingBack, events.TypeRecoveryFailed, StatusFailed, []string{events.TypeHumanInterventionRequested}},
		// Running → Recovering：Action 失败（可恢复）
		{StatusRunning, events.TypeActionFailed, StatusRecovering, []string{events.TypeActionRetrying}},
		// Recovering → Running：恢复成功
		{StatusRecovering, events.TypeActionCompleted, StatusRunning, nil},
		// Recovering → Failed：恢复失败
		{StatusRecovering, events.TypeRecoveryFailed, StatusFailed, []string{events.TypeHumanInterventionRequested}},
	}

	for _, r := range matrix {
		if r.from == current && r.evt == evtType {
			return r.to, r.emit, true
		}
	}
	// 未匹配到转换规则——状态不变
	return current, nil, false
}

// UserVisible 将内部状态映射为用户可见状态。
// 三个用户可见状态：🟢 进行中 / 🟡 需要处理 / ✅ 已完成。
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
