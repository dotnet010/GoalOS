// Package llm 提供 GoalOS 的 LLM 抽象层。
// 包含共享类型、错误分类、重试逻辑和结构化输出定义。
// 所有 LLM Provider（Cloud API、Ollama、Anthropic）均通过此包定义的接口访问。
//
// 设计依据：05 架构文档 §A.8、R242、ADR-014。
package llm

import "math"

// ChatRequest 是一次 LLM 对话请求。
type ChatRequest struct {
	Messages   []Message // 对话消息列表（System + User）
	MaxTokens  int       // 最大生成 Token 数
	ToolChoice string    // "auto" | "required" | "none"。默认 "auto"
}

// ChatResponse 是一次 LLM 对话响应。
type ChatResponse struct {
	Content   string     // 响应文本（纯文本或 Tool Call JSON）
	ToolCalls []ToolCall // Function Calling 返回的 Tool Call 列表
	Usage     TokenUsage // Token 消耗统计
}

// Message 是一条对话消息。
type Message struct {
	Role    string // "system" | "user" | "assistant"
	Content string // 消息内容
}

// ToolCall 表示一次 Function Calling 调用。
type ToolCall struct {
	Name      string // 被调用的函数名
	Arguments string // JSON 格式的函数参数
}

// TokenUsage 记录一次 LLM 调用的 Token 消耗。
type TokenUsage struct {
	PromptTokens     int // 输入 Token 数
	CompletionTokens int // 输出 Token 数
	TotalTokens      int // 总 Token 数
}

// SafeTemperature 处理 Temperature 零值问题。
// go-openai 库中 Temperature float32 字段有 omitempty 标签，
// Temperature=0 会被 JSON 序列化省略，导致 OpenAI 默认使用 Temperature=1。
// 使用 math.SmallestNonzeroFloat32 代替 0，确保近似确定性输出。
//
// 参考：https://github.com/sashabaranov/go-openai/issues/9
func SafeTemperature(t float32) float32 {
	if t <= 0 {
		return math.SmallestNonzeroFloat32
	}
	return t
}
