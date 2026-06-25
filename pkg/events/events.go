// Package events 定义 GoalOS 所有系统事件类型和基础 Event 结构体。
// 这是事件定义的唯一权威来源——所有模块引用此包。
//
// 设计依据：05 架构文档 附录 A.7、07 事件注册表。
package events

import (
	"fmt"
	"time"
)

// NewEvent 创建一个带有必填字段的新 Event。
// 自动填充 Version、Timestamp。调用方需设置 Seq、Payload。
func NewEvent(typ, goalID, source string) Event {
	return Event{
		Type:      typ,
		Version:   "1.0",
		GoalID:    goalID,
		Source:    source,
		Timestamp: time.Now(),
	}
}

// WithAction 设置 Action 相关字段并返回 Event。
// 自动生成幂等键：{goalID}-{actionID}-{seq}。
func (e Event) WithAction(actionID string) Event {
	e.ActionID = &actionID
	e.ID = fmt.Sprintf("%s-%s-%d", e.GoalID, actionID, e.Seq)
	return e
}

// WithPayload 设置 Payload 并返回 Event。
func (e Event) WithPayload(p map[string]interface{}) Event {
	e.Payload = p
	return e
}

// Event 是所有系统事件的基础结构体。
// 每个事件持久化到 events.jsonl，是 Source of Truth。
type Event struct {
	Seq       int                    `json:"seq"`              // 全局递增序号
	Type      string                 `json:"type"`             // 事件类型。见下方常量
	Version   string                 `json:"version"`          // 事件版本。"1.0"
	ID        string                 `json:"idempotency_key"`  // 幂等键：{goalID}-{actionID}-{seq}。重放时去重
	Timestamp time.Time              `json:"timestamp"`        // 事件时间戳
	GoalID    string                 `json:"goal_id"`          // 所属 Goal
	MissionID *string                `json:"mission_id,omitempty"` // 所属 Mission。可选
	ActionID  *string                `json:"action_id,omitempty"`  // 所属 Action。可选
	Source    string                 `json:"source"`           // 发布者模块名
	Payload   map[string]interface{} `json:"payload"`          // 事件特定数据
	Metadata  map[string]interface{} `json:"metadata,omitempty"` // 元数据。可选
}

// ─── 事件类型常量 ──────────────────────────────────────────────

const (
	// ── Goal 生命周期 ──
	TypeGoalCreated      = "GoalCreated"      // 用户创建新 Goal。Publisher: daemon
	TypeMissionGenerated = "MissionGenerated" // Agent 产出 MissionGraph。Publisher: Mission Engine
	TypeUserConfirmed    = "UserConfirmed"    // 用户确认 MissionGraph。Publisher: Channel Adapter
	TypeUserRejected     = "UserRejected"     // 用户拒绝 MissionGraph。Publisher: Channel Adapter
	TypeGoalCompleted    = "GoalCompleted"    // Goal 完成。Publisher: Scheduler
	TypeGoalPaused       = "GoalPaused"       // Goal 已暂停。Publisher: Scheduler
	TypeGoalResumed      = "GoalResumed"      // Goal 已恢复。Publisher: Scheduler

	// ── 规划调度 ──
	TypePlanRequested = "PlanRequested" // Scheduler 请求 Mission Engine 规划。Publisher: Scheduler

	// ── Action 调度 ──
	TypeActionScheduled = "ActionScheduled" // Scheduler 选出下一个 Action。Publisher: Scheduler

	// ── Governance 决策 ──
	TypeActionApproved        = "ActionApproved"        // Governance 批准。Publisher: Governance
	TypeActionRejected        = "ActionRejected"        // Governance 拒绝。Publisher: Governance
	TypeActionPendingApproval = "ActionPendingApproval" // 挂起等待人工审批。Publisher: Governance
	TypeUserApprovedAction    = "UserApprovedAction"    // 用户批准挂起的审批。Publisher: Channel Adapter

	// ── 执行与结果 ──
	TypeActionStarted   = "ActionStarted"   // 子进程开始执行。Publisher: Plugin Runner
	TypeActionCompleted = "ActionCompleted" // 执行成功。Publisher: Plugin Runner
	TypeActionFailed    = "ActionFailed"    // 执行失败。Publisher: Plugin Runner
	TypeActionCancelled = "ActionCancelled" // 执行被取消。Publisher: Scheduler

	// ── Recovery ──
	TypeActionRetryScheduled       = "ActionRetryScheduled"       // 定时重试。Publisher: Scheduler 定时器
	TypeActionRetrying             = "ActionRetrying"             // 正在重试。Publisher: Scheduler
	TypeActionRolledBack           = "ActionRolledBack"           // 已回滚。Publisher: Scheduler
	TypeActionReplanned            = "ActionReplanned"            // 已重规划。Publisher: Scheduler
	TypeHumanInterventionRequested = "HumanInterventionRequested" // 需要人工介入。Publisher: Scheduler
	TypeRecoveryFailed             = "RecoveryFailed"             // Recovery 失败。Publisher: Scheduler

	// ── 验证循环 ──
	TypeVerificationResult      = "VerificationResult"      // 验收结果。Publisher: Plugin Runner
	TypeVerificationFailed      = "VerificationFailed"      // 验收失败。Publisher: Scheduler
	TypeSelfCorrectionExhausted = "SelfCorrectionExhausted" // 自修正耗尽。Publisher: Scheduler

	// ── 用户请求事件 ──
	TypeGoalPauseRequested    = "GoalPauseRequested"    // 用户请求暂停。Publisher: Channel Adapter
	TypeGoalRollbackRequested = "GoalRollbackRequested" // 用户请求回滚。Publisher: Channel Adapter
	TypeGoalStopRequested     = "GoalStopRequested"     // 用户请求终止。Publisher: Channel Adapter

	// ── 产出物与经验 ──
	TypeArtifactCreated     = "ArtifactCreated"     // 新文件产出。Publisher: Plugin Runner
	TypeExperienceGenerated = "ExperienceGenerated" // 经验文件生成。Publisher: Context Engine
	TypePatternExtracted    = "PatternExtracted"    // 跨 Goal 模式提炼。Publisher: Context Engine

	// ── 快照与系统 ──
	TypeSnapshotCreated      = "SnapshotCreated"      // 快照写入。Publisher: State Store
	TypeTokenUsage           = "TokenUsage"           // Token 消耗记录。Publisher: Scheduler
	TypeSystemStarted        = "SystemStarted"        // Daemon 启动完成。Publisher: daemon
	TypeMissionGraphRejected = "MissionGraphRejected" // MissionGraph 校验失败。Publisher: Mission Engine

	// ── 消息 ──
	TypeMessageReceived = "MessageReceived" // 收到用户消息。Publisher: Channel Adapter
	TypeMessageSent     = "MessageSent"     // 发送回复。Publisher: Channel Adapter

	// ── v1.1.0 Primitive 事件 ──
	TypeCheckPerformed               = "CheckPerformed"               // Check 原语完成。Publisher: PipelineRunner
	TypeGateEvaluated                = "GateEvaluated"                // Gate 评估完成。Publisher: PipelineRunner
	TypeDecidePathSelected           = "DecidePathSelected"           // Decide 路径选择。Publisher: PipelineRunner
	TypePipelinePaused               = "PipelinePaused"               // Wait 暂停。Publisher: PipelineRunner
	TypePipelineResumed              = "PipelineResumed"              // Wait 恢复。Publisher: PipelineRunner
	TypeFlowStarted                  = "FlowStarted"                  // Flow 开始。Publisher: PipelineRunner
	TypeFlowCompleted                = "FlowCompleted"                // Flow 完成。Publisher: PipelineRunner
	TypeMultiLLMVerificationCompleted = "MultiLLMVerificationCompleted" // Multi-LLM 裁决完成。Publisher: Plugin Runner
	TypeInvariantViolated            = "InvariantViolated"            // 运行时不变式违反。Publisher: 各核心模块
	TypeTaskAnalysisCompleted        = "TaskAnalysisCompleted"        // Agent.Analyze() 完成。Publisher: Mission Engine
	TypeResourceAvailable            = "ResourceAvailable"            // 资源可用。Publisher: Plugin Runner / OS Monitor
	TypeDataSharingApproved          = "DataSharingApproved"          // 用户批准外发数据。Publisher: Channel Adapter

	// ── Plugin 生命周期（v2 预留）──
	TypePluginProcessTerminated = "PluginProcessTerminated" // Plugin 进程终止。Publisher: Plugin Runner
	TypeCapabilityRegistered    = "CapabilityRegistered"    // 动态能力注册。Publisher: Plugin Runner (v2)
	TypeCapabilityRevoked       = "CapabilityRevoked"       // 能力撤销。Publisher: Plugin Runner (v2)
)
