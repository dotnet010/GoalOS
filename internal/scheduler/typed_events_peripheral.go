// Package scheduler — v0.2.0 Week 1: 外围事件 typed payload（11个）。
// v0.2.0 W1: 全部实现 EventPayload 接口（EventType + Validate）。
package scheduler

import (
	"fmt"
	"time"

	"github.com/goalos/goalos/internal/kernel"
)

// 编译期验证：所有外围 typed payload 实现 EventPayload 接口。
var (
	_ kernel.EventPayload = GoalPausedPayload{}
	_ kernel.EventPayload = GoalResumedPayload{}
	_ kernel.EventPayload = ActionApprovedPayload{}
	_ kernel.EventPayload = ActionRejectedPayload{}
	_ kernel.EventPayload = PipelinePausedPayload{}
	_ kernel.EventPayload = PipelineResumedPayload{}
	_ kernel.EventPayload = HumanInterventionRequestedPayload{}
	_ kernel.EventPayload = PluginRegisteredPayload{}
	_ kernel.EventPayload = PluginUnregisteredPayload{}
	_ kernel.EventPayload = SessionCreatedPayload{}
	_ kernel.EventPayload = TokenBudgetAdjustedPayload{}
)

// ─── 生命周期事件 ────────────────────────────────────────────

// GoalPausedPayload Goal 暂停事件。
type GoalPausedPayload struct {
	GoalID   string `json:"goal_id"`
	PausedAt string `json:"paused_at"`
	Source   string `json:"source"` // "user" | "system"
}

func (p GoalPausedPayload) EventType() string { return "GoalPaused" }
func (p GoalPausedPayload) Validate() error {
	if p.GoalID == "" {
		return fmt.Errorf("GoalPausedPayload: GoalID is required")
	}
	return nil
}

// GoalResumedPayload Goal 恢复事件。
type GoalResumedPayload struct {
	GoalID    string `json:"goal_id"`
	ResumedAt string `json:"resumed_at"`
	Source    string `json:"source"`
}

func (p GoalResumedPayload) EventType() string { return "GoalResumed" }
func (p GoalResumedPayload) Validate() error {
	if p.GoalID == "" {
		return fmt.Errorf("GoalResumedPayload: GoalID is required")
	}
	return nil
}

// ─── Governance 事件 ─────────────────────────────────────────

// ActionApprovedPayload Action 审批通过。
type ActionApprovedPayload struct {
	ActionID     string `json:"action_id"`
	PolicyResult string `json:"policy_result"`
	RiskLevel    string `json:"risk_level"`
	TokenID      string `json:"token_id"`
}

func (p ActionApprovedPayload) EventType() string { return "ActionApproved" }
func (p ActionApprovedPayload) Validate() error {
	if p.ActionID == "" {
		return fmt.Errorf("ActionApprovedPayload: ActionID is required")
	}
	if p.TokenID == "" {
		return fmt.Errorf("ActionApprovedPayload: TokenID is required — 审批通过必须持有有效 Token")
	}
	return nil
}

// ActionRejectedPayload Action 被拒绝。
type ActionRejectedPayload struct {
	ActionID     string `json:"action_id"`
	RejectReason string `json:"reject_reason"` // "policy_denied" | "capability_denied" | "risk_rejected" | "approval_denied"
	RejectSource string `json:"reject_source"` // "policy" | "capability" | "risk" | "approval"
}

func (p ActionRejectedPayload) EventType() string { return "ActionRejected" }
func (p ActionRejectedPayload) Validate() error {
	if p.ActionID == "" {
		return fmt.Errorf("ActionRejectedPayload: ActionID is required")
	}
	validReasons := map[string]bool{
		"policy_denied": true, "capability_denied": true,
		"risk_rejected": true, "approval_denied": true,
	}
	if !validReasons[p.RejectReason] {
		return fmt.Errorf("ActionRejectedPayload: invalid RejectReason '%s'", p.RejectReason)
	}
	return nil
}

// ─── Pipeline 事件 ──────────────────────────────────────────

// PipelinePausedPayload PipelineRunner Wait 暂停。
type PipelinePausedPayload struct {
	GoalID       string         `json:"goal_id"`
	ActionID     string         `json:"action_id"`
	PendingWaits []WaitCondition `json:"pending_waits"` // R-764
	TimeoutAt    time.Time      `json:"timeout_at"`
}

func (p PipelinePausedPayload) EventType() string { return "PipelinePaused" }
func (p PipelinePausedPayload) Validate() error {
	if p.GoalID == "" {
		return fmt.Errorf("PipelinePausedPayload: GoalID is required")
	}
	return nil
}

// PipelineResumedPayload PipelineRunner Wait 恢复。
type PipelineResumedPayload struct {
	GoalID         string `json:"goal_id"`
	ActionID       string `json:"action_id"`
	ResumedReason  string `json:"resumed_reason"`
	WaitDurationMs int    `json:"wait_duration_ms"`
}

func (p PipelineResumedPayload) EventType() string { return "PipelineResumed" }
func (p PipelineResumedPayload) Validate() error {
	if p.GoalID == "" {
		return fmt.Errorf("PipelineResumedPayload: GoalID is required")
	}
	return nil
}

// ─── 人工干预 ────────────────────────────────────────────────

// HumanInterventionRequestedPayload 需要用户介入。
type HumanInterventionRequestedPayload struct {
	GoalID             string   `json:"goal_id"`
	FailedActionID     string   `json:"failed_action_id"`
	NewSessionAttempts int      `json:"new_session_attempts"`
	Reason             string   `json:"reason"`
	ResumeOptions      []string `json:"resume_options"` // R-800: ["继续等待", "ESCALATE"]
}

func (p HumanInterventionRequestedPayload) EventType() string { return "HumanInterventionRequested" }
func (p HumanInterventionRequestedPayload) Validate() error {
	if p.GoalID == "" {
		return fmt.Errorf("HumanInterventionRequestedPayload: GoalID is required")
	}
	if p.FailedActionID == "" {
		return fmt.Errorf("HumanInterventionRequestedPayload: FailedActionID is required")
	}
	return nil
}

// ─── Plugin 事件 ─────────────────────────────────────────────

// PluginRegisteredPayload Plugin 注册完成。
type PluginRegisteredPayload struct {
	PluginID     string   `json:"plugin_id"`
	PluginName   string   `json:"plugin_name"`
	PluginType   string   `json:"plugin_type"`
	Version      string   `json:"version"`
	Capabilities []string `json:"capabilities"`
	Signature    string   `json:"signature"`
	BinaryPath   string   `json:"binary_path"`
	RegisteredAt string   `json:"registered_at"`
}

func (p PluginRegisteredPayload) EventType() string { return "PluginRegistered" }
func (p PluginRegisteredPayload) Validate() error {
	if p.PluginID == "" {
		return fmt.Errorf("PluginRegisteredPayload: PluginID is required")
	}
	if p.Signature == "" {
		return fmt.Errorf("PluginRegisteredPayload: Signature is required — 未签名插件不得注册")
	}
	return nil
}

// PluginUnregisteredPayload Plugin 移除。
type PluginUnregisteredPayload struct {
	PluginID       string `json:"plugin_id"`
	PluginName     string `json:"plugin_name"`
	Reason         string `json:"reason"`
	UnregisteredAt string `json:"unregistered_at"`
}

func (p PluginUnregisteredPayload) EventType() string { return "PluginUnregistered" }
func (p PluginUnregisteredPayload) Validate() error {
	if p.PluginID == "" {
		return fmt.Errorf("PluginUnregisteredPayload: PluginID is required")
	}
	return nil
}

// ─── 系统事件 ────────────────────────────────────────────────

// SessionCreatedPayload 新 Session 创建。
type SessionCreatedPayload struct {
	SessionID       string `json:"session_id"`
	GoalID          string `json:"goal_id"`
	ParentSessionID string `json:"parent_session_id,omitempty"`
	ResumePoint     string `json:"resume_point"`
}

func (p SessionCreatedPayload) EventType() string { return "SessionCreated" }
func (p SessionCreatedPayload) Validate() error {
	if p.SessionID == "" {
		return fmt.Errorf("SessionCreatedPayload: SessionID is required")
	}
	if p.GoalID == "" {
		return fmt.Errorf("SessionCreatedPayload: GoalID is required")
	}
	return nil
}

// TokenBudgetAdjustedPayload Token 预算追加。
type TokenBudgetAdjustedPayload struct {
	GoalID         string `json:"goal_id"`
	PreviousBudget int    `json:"previous_budget"`
	AddedAmount    int    `json:"added_amount"`
	NewBudget      int    `json:"new_budget"`
	AdjustedAt     string `json:"adjusted_at"`
}

func (p TokenBudgetAdjustedPayload) EventType() string { return "TokenBudgetAdjusted" }
func (p TokenBudgetAdjustedPayload) Validate() error {
	if p.GoalID == "" {
		return fmt.Errorf("TokenBudgetAdjustedPayload: GoalID is required")
	}
	if p.AddedAmount <= 0 {
		return fmt.Errorf("TokenBudgetAdjustedPayload: AddedAmount must be positive, got %d", p.AddedAmount)
	}
	return nil
}
