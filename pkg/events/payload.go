// Package events — v0.2.0 Week 1: typed event payload + Validatable 接口
// M1-M8 Validate() 实现。EventBus.Publish() 自动调用（R-770）。
package events

import (
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
		return fmt.Errorf("Title: nonempty")
	}
	// M6: len < 10000
	if len(p.Title) > 10000 {
		return fmt.Errorf("Title: len<10000 (actual=%d)", len(p.Title))
	}
	// M6: UTF-8
	if !utf8.ValidString(p.Title) {
		return fmt.Errorf("Title: utf8")
	}
	// M6: 无 HTML 标签
	for i := 0; i < len(p.Title)-1; i++ {
		if p.Title[i] == '<' {
			for j := i + 1; j < len(p.Title); j++ {
				if p.Title[j] == '>' {
					return fmt.Errorf("Title: no_html (found <%s>)", p.Title[i+1:j])
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
		return fmt.Errorf("Type: invalid value %q (expected result|error)", p.Type)
	}
	// M3: status 枚举检查
	if p.Status != "success" && p.Status != "failure" {
		return fmt.Errorf("Status: invalid value %q (expected success|failure)", p.Status)
	}
	// M3: action_id 非空
	if p.ActionID == "" {
		return fmt.Errorf("ActionID: nonempty")
	}
	// M3: output ≤ 64KB
	if len(p.Output) > 64*1024 {
		return fmt.Errorf("Output: len<=64KB (actual=%d)", len(p.Output))
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
		return fmt.Errorf("Size: len<=%d (actual=%d)", maxSize, p.Size)
	}
	return nil
}
