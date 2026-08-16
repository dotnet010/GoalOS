// Package events — v0.2.0 Week 1: typed event payload + Validatable 接口
// M1-M8 Validate() 实现。EventBus.Publish() 自动调用（R-770）。
package events

import (
	"encoding/json"
	"fmt"
	"unicode/utf8"
)

// ─── Validatable 接口（H8 + R-770）─────────────────────────────

// Validatable 由所有跨模块传递的 event payload 实现。
// EventBus.Publish() 在投递事件前自动调用 Validate()。
type Validatable interface {
	Validate() error
}

// ─── M1 + M6: GoalCreatedPayload ──────────────────────────────────

// GoalCreatedPayload 是 GoalCreated 事件的 payload。
type GoalCreatedPayload struct {
	GoalID      string `json:"goal_id"`      // M1: MUST 非空
	Title       string `json:"title"`        // 用户输入
	Description string `json:"description"`  // 可选
	Tags        []string `json:"tags"`       // 可选
}

func (p GoalCreatedPayload) Validate() error {
	// M1: GoalID 非空
	if p.GoalID == "" {
		return fmt.Errorf("GoalID: nonempty")
	}
	// M1: GoalID 不能仅含空白
	for _, r := range p.GoalID {
		if r != ' ' && r != '\t' && r != '\n' {
			goto validGoalID
		}
	}
	return fmt.Errorf("GoalID: nonempty (whitespace-only)")
validGoalID:

	// M6: goal 非空
	if p.Title == "" {
		return fmt.Errorf("title: nonempty")
	}
	// M6: len < 10000
	if len(p.Title) > 10000 {
		return fmt.Errorf("title: len<10000 (actual=%d)", len(p.Title))
	}
	// M6: UTF-8
	if !utf8.ValidString(p.Title) {
		return fmt.Errorf("title: utf8")
	}
	// M6: 无 HTML 标签
	for i := 0; i < len(p.Title)-1; i++ {
		if p.Title[i] == '<' {
			for j := i + 1; j < len(p.Title); j++ {
				if p.Title[j] == '>' {
					return fmt.Errorf("title: no_html (found <%s>)", p.Title[i+1:j])
				}
			}
		}
	}
	return nil
}

// ─── M2: CompletionCriteria ────────────────────────────────────────

// CompletionCriteria 是 Agent.Align() 的输出。
type CompletionCriteria struct {
	GoalType          string `json:"goal_type"`
	SuccessDefinition string `json:"success_definition"`
}

func (p CompletionCriteria) Validate() error {
	// M2: goal_type 非空
	if p.GoalType == "" {
		return fmt.Errorf("GoalType: nonempty")
	}
	// M2: SuccessDefinition 非空
	if p.SuccessDefinition == "" {
		return fmt.Errorf("SuccessDefinition: nonempty")
	}
	// M2: goal_type 合法值检查
	validTypes := map[string]bool{
		"code_generation": true, "data_analysis": true, "research": true,
		"content_creation": true, "automation": true, "generic": true,
	}
	if !validTypes[p.GoalType] {
		return fmt.Errorf("GoalType: invalid value %q (expected one of: code_generation, data_analysis, research, content_creation, automation, generic)", p.GoalType)
	}
	return nil
}

// ─── M3: IPCResultPayload ──────────────────────────────────────────

// IPCResultPayload 是 Plugin 子进程返回的消息 payload。
type IPCResultPayload struct {
	Type     string `json:"type"`      // "result" | "error"
	ActionID string `json:"action_id"`
	Status   string `json:"status"`    // "success" | "failure"
	Output   string `json:"output"`
}

func (p IPCResultPayload) Validate() error {
	// M3: type 枚举检查
	if p.Type != "result" && p.Type != "error" {
		return fmt.Errorf("type: invalid value %q (expected result|error)", p.Type)
	}
	// M3: status 枚举检查
	if p.Status != "success" && p.Status != "failure" {
		return fmt.Errorf("status: invalid value %q (expected success|failure)", p.Status)
	}
	// M3: action_id 非空
	if p.ActionID == "" {
		return fmt.Errorf("actionID: nonempty")
	}
	// M3: output ≤ 64KB
	if len(p.Output) > 64*1024 {
		return fmt.Errorf("output: len<=64KB (actual=%d)", len(p.Output))
	}
	return nil
}

// ─── M4 + M8: GoalCompletedPayload ─────────────────────────────────

// GoalCompletedPayload 是 GoalCompleted 事件的 payload。
type GoalCompletedPayload struct {
	GoalID       string `json:"goal_id"`
	ArtifactPath string `json:"artifact_path"`
	GoalState    string `json:"goal_state"` // 当前 Goal 状态——用于 M8 防重复
}

func (p GoalCompletedPayload) Validate() error {
	// M4: artifact_path 非空
	if p.ArtifactPath == "" {
		return fmt.Errorf("ArtifactPath: nonempty")
	}
	// M8: Goal 状态不能为 Failed（防重复发布）
	if p.GoalState == "Failed" {
		return fmt.Errorf("GoalState: cannot be Failed——GoalCompleted 不可在 GoalFailed 后发布")
	}
	return nil
}

// ─── M7: FileContentPayload ────────────────────────────────────────

// FileContentPayload 是 ContextEngine 读取文件时的 payload。
type FileContentPayload struct {
	Path string `json:"path"`
	Size int64  `json:"size"`
}

func (p FileContentPayload) Validate() error {
	// M7: 文件大小 ≤ 10MB
	const maxSize int64 = 10 * 1024 * 1024
	if p.Size > maxSize {
		return fmt.Errorf("size: len<=%d (actual=%d)", maxSize, p.Size)
	}
	return nil
}

// PayloadToMap 将 typed payload 转换为 map[string]interface{}（R-828 统一）。
func PayloadToMap(v interface{}) map[string]interface{} {
	switch p := v.(type) {
	case ActionScheduledPayload:
		return map[string]interface{}{
			"action_id": p.ActionID, "action_type": p.ActionType, "source": p.Source,
			"timeout_seconds": p.TimeoutSeconds, "risk_level_pre": p.RiskLevelPre,
			"required_capabilities": p.RequiredCapabilities, "target": p.Target,
		}
	case ActionCompletedPayload:
		return map[string]interface{}{
			"action_id": p.ActionID, "status": p.Status, "output": p.Output,
			"artifacts_produced": p.ArtifactsProduced, "duration_ms": p.DurationMs, "tokens": p.Tokens,
		}
	case GoalCompletedPayloadV2:
		return map[string]interface{}{
			"goal_id": p.GoalID, "artifact_path": p.ArtifactPath, "goal_state": p.GoalState,
			"duration_seconds": p.DurationSeconds, "total_actions": p.TotalActions,
			"succeeded_actions": p.SucceededActions, "failed_actions": p.FailedActions,
			"total_tokens": p.TotalTokens, "human_interventions": p.HumanInterventions,
		}
	case GoalFailedPayload:
		return map[string]interface{}{
			"goal_id": p.GoalID, "reason": p.Reason,
			"error": p.Error, "error_hint": p.ErrorHint,
		}
	default:
		data, _ := json.Marshal(v)
		var result map[string]interface{}
		json.Unmarshal(data, &result)
		return result
	}
}

// ─── R-828 Step 1: 从 internal/scheduler 迁移核心 payload ──────────

// ActionScheduledPayload 是 ActionScheduled 事件的 typed payload。
type ActionScheduledPayload struct {
	ActionID             string   `json:"action_id"`
	ActionType           string   `json:"action_type,omitempty"`
	Target               string   `json:"target,omitempty"`
	Source               string   `json:"source,omitempty"`
	RequiredCapabilities []string `json:"required_capabilities,omitempty"`
	TimeoutSeconds       int      `json:"timeout_seconds"`
	RiskLevelPre         string   `json:"risk_level_pre,omitempty"`
}

func (p ActionScheduledPayload) EventType() string { return "ActionScheduled" }
func (p ActionScheduledPayload) Validate() error {
	if p.ActionID == "" {
		return fmt.Errorf("ActionScheduledPayload: ActionID is required")
	}
	return nil
}

// ActionCompletedPayload 是 ActionCompleted 事件的 typed payload。
type ActionCompletedPayload struct {
	ActionID          string   `json:"action_id"`
	Status            string   `json:"status"`
	Output            string   `json:"output,omitempty"`
	ArtifactsProduced []string `json:"artifacts_produced,omitempty"`
	DurationMs        int      `json:"duration_ms"`
	Tokens            int      `json:"tokens"`
}

func (p ActionCompletedPayload) EventType() string { return "ActionCompleted" }
func (p ActionCompletedPayload) Validate() error {
	if p.ActionID == "" {
		return fmt.Errorf("ActionCompletedPayload: ActionID is required")
	}
	if p.Status != "success" && p.Status != "failure" {
		return fmt.Errorf("ActionCompletedPayload: Status must be success|failure, got %q", p.Status)
	}
	return nil
}

// GoalFailedPayload 是 GoalFailed 事件的 typed payload。
type GoalFailedPayload struct {
	GoalID    string `json:"goal_id"`
	Reason    string `json:"reason"`
	Error     string `json:"error,omitempty"`
	ErrorHint string `json:"error_hint,omitempty"`
}

func (p GoalFailedPayload) EventType() string { return "GoalFailed" }
func (p GoalFailedPayload) Validate() error {
	if p.GoalID == "" {
		return fmt.Errorf("GoalFailedPayload: GoalID is required")
	}
	if p.Reason == "" {
		return fmt.Errorf("GoalFailedPayload: Reason is required")
	}
	return nil
}

// GoalCompletedPayloadV2 是 GoalCompleted 事件的增强 typed payload（R-828）。
type GoalCompletedPayloadV2 struct {
	GoalID             string `json:"goal_id"`
	ArtifactPath       string `json:"artifact_path"`
	GoalState          string `json:"goal_state,omitempty"`
	DurationSeconds    int    `json:"duration_seconds"`
	TotalActions       int    `json:"total_actions"`
	SucceededActions   int    `json:"succeeded_actions"`
	FailedActions      int    `json:"failed_actions"`
	TotalTokens        int    `json:"total_tokens"`
	HumanInterventions int    `json:"human_interventions"`
}

func (p GoalCompletedPayloadV2) EventType() string { return "GoalCompleted" }
func (p GoalCompletedPayloadV2) Validate() error {
	if p.GoalID == "" {
		return fmt.Errorf("GoalCompletedPayloadV2: GoalID is required")
	}
	if p.ArtifactPath == "" {
		return fmt.Errorf("GoalCompletedPayloadV2: ArtifactPath is required")
	}
	if p.GoalState == "Failed" {
		return fmt.Errorf("GoalCompletedPayloadV2: GoalState cannot be Failed")
	}
	return nil
}
