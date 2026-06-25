// Package scheduler — Persona v1.1.0 集成。
// PromptBuilder: 推理层 vs 渲染层 prompt 构建。
// Immutable Layer 0: 不可变系统提示词约束。
// Level 0.5: 分歧事实层。
//
// 设计依据：05 架构文档 §4.3.1、R348。

package scheduler

import (
	"fmt"
	"strings"
)

// PromptBuilder 构建 LLM prompt（v1.1.0）。
// 区分推理层（中性客观）和渲染层（含风格人设）。
type PromptBuilder struct {
	immutableLayer0 string // 不可变系统提示词约束
}

// NewPromptBuilder 创建 PromptBuilder。加载 Immutable Layer 0。
func NewPromptBuilder() *PromptBuilder {
	return &PromptBuilder{
		immutableLayer0: immutableLayer0Constraints,
	}
}

// BuildReasoningPrompt 构建推理层 prompt（中性客观。不含风格人设）。
// 自动注入 Immutable Layer 0 约束。
func (pb *PromptBuilder) BuildReasoningPrompt(role string, taskContext map[string]string) string {
	var b strings.Builder

	// Immutable Layer 0: 不可变系统提示词（所有推理层 prompt 强制注入）
	b.WriteString(pb.immutableLayer0)
	b.WriteString("\n\n")

	// 角色上下文
	if role != "" {
		b.WriteString("## 角色: " + role + "\n")
	}
	for k, v := range taskContext {
		b.WriteString(k + ": " + v + "\n")
	}
	return b.String()
}

// BuildRenderingPrompt 构建渲染层 prompt（含风格人设。用于 Channel 消息）。
func (pb *PromptBuilder) BuildRenderingPrompt(eventType string, personaStyle string, context map[string]string) string {
	var b strings.Builder

	// 风格人设
	switch personaStyle {
	case "concise":
		b.WriteString("你是 GoalOS 的简洁助手。用最少的话传达关键信息。\n")
	case "warm":
		b.WriteString("你是 GoalOS 的温暖助手。用鼓励的语气帮助用户。\n")
	case "minimal":
		b.WriteString("极简。只给事实。\n")
	default:
		b.WriteString("你是 GoalOS。\n")
	}
	b.WriteString("\n")

	// 事件类型 + 上下文
	b.WriteString("事件: " + eventType + "\n")
	for k, v := range context {
		b.WriteString(k + ": " + v + "\n")
	}

	// Level 0 安全约束（渲染层也需遵守——不可变事实禁止修改）
	b.WriteString("\n## 消息约束\n")
	b.WriteString("- 不可变事实禁止修改（目标名称、风险等级、操作描述、统计数据）\n")
	b.WriteString("- 审批消息禁止 emoji\n")
	b.WriteString("- 不假装有情绪、有生活、有对用户个人的持久记忆\n")

	return b.String()
}

// BuildDivergenceNotice 构建 Multi-LLM 分歧通知（v1.1.0 Level 0.5）。
// 信息结构由 Core 固定（不可被 Persona 重组），措辞语气由 Level 3 控制。
func (pb *PromptBuilder) BuildDivergenceNotice(verdict *Verdict, style string) string {
	var b strings.Builder

	// Level 0.5: 信息结构固定
	b.WriteString("⚠️ Multi-LLM 验证存在分歧\n\n")
	b.WriteString("裁决结果: " + verdict.Result + "\n")
	b.WriteString("加权分数: " + formatFloat(verdict.WeightedScore) + "\n\n")

	b.WriteString("各 Provider 投票:\n")
	for _, v := range verdict.Votes {
		b.WriteString("- " + v.Provider + "/" + v.Model + ": **" + v.Vote + "**")
		if v.Reasoning != "" {
			shortReason := truncate(v.Reasoning, 100)
			b.WriteString(" (" + shortReason + ")")
		}
		b.WriteString("\n")
	}

	// Level 3: 措辞语气由 Persona 控制
	switch style {
	case "warm":
		b.WriteString("\n建议: 验证结果不完全一致。系统将继续执行，但您可以随时暂停检查。")
	case "minimal":
		b.WriteString("\n分歧。继续执行。可暂停。")
	default: // concise
		b.WriteString("\n系统判定: 继续执行但建议关注。")
	}

	return b.String()
}

func formatFloat(f float64) string {
	return strings.TrimRight(strings.TrimRight(
		strings.Replace(fmt.Sprintf("%.2f", f), ".00", "", 1),
		"0"), ".")
}

// immutableLayer0Constraints 是 Immutable Layer 0 的完整约束文本。
// 所有推理层 prompt 强制注入。不被 Persona/Flow/Skill 覆盖。
const immutableLayer0Constraints = `## 不可变约束（Immutable Layer 0）

1. 目标完整性不可侵犯——绝对不能建议用户降低目标、接受部分完成、或放弃 must_have 标准
2. 建议必须指向完成原始目标——"部分完成"不是有效输出
3. 区分"用户改变目标"和"系统建议改变目标"——用户有权改变。系统绝不能主动建议放弃
4. 诚实但不放弃——如果遇到困难，诚实告知并尝试替代方案，但绝不建议放弃目标本身
`
