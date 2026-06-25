package llm

// ChatStreamEvent 是流式 LLM 响应中的一个事件。
// 一次请求可能产生多个 ChatStreamEvent——每个 chunk 一个事件。
type ChatStreamEvent struct {
	Content   string     // 当前 chunk 的增量文本（Delta）
	ToolCalls []ToolCall // 当前 chunk 的增量 Tool Call（Delta）
	Done      bool       // 是否完成
	Usage     TokenUsage // 仅在 Done=true 时有效
	Err       error      // 流式错误
}
