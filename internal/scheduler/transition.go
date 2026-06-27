// Package scheduler — 核心类型定义。
// GoalStatus 和 ActionStatus 是 v0.0.x 保留类型——v0.1.0 GoalRunner+PipelineRunner 依赖。
// Transition() 函数已被 PipelineRunner 替代（v0.1.0）。UserVisible() 合并入 Persona 渲染层。
package scheduler

// GoalStatus 是 Goal 的内部状态。
type GoalStatus string

const (
	StatusDraft       GoalStatus = "draft"
	StatusPlanned     GoalStatus = "planned"
	StatusRunning     GoalStatus = "running"
	StatusPaused      GoalStatus = "paused"
	StatusRecovering  GoalStatus = "recovering"
	StatusCompleted   GoalStatus = "completed"
	StatusFailed      GoalStatus = "failed_terminal"
	StatusRollingBack GoalStatus = "rolling_back"
)

// ActionStatus 是 Action 级状态。Scheduler 唯一拥有和维护。
type ActionStatus string

const (
	ActionPending    ActionStatus = "pending"
	ActionScheduled  ActionStatus = "scheduled"
	ActionApproved   ActionStatus = "approved"
	ActionExecuting  ActionStatus = "executing"
	ActionVerifying  ActionStatus = "verifying"
	ActionCompleted  ActionStatus = "completed"
	ActionFailed     ActionStatus = "failed"
	ActionRecovering ActionStatus = "recovering"
)
