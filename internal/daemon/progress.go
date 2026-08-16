// progress.go — ProgressMessage 实现（任务 7.26——D49⑤ 会议 #198）。
//
// 契约（08 §3.2 消息类型表）：{type:"progress", action_id, progress:{completed_tokens,
// total_tokens_estimate, phase, message?}}；发送频率=每 500 token 或每 5s 取先到
// （policy.progress_interval_tokens 透传）。
package daemon

// ProgressMessage — 进度消息（08 §3.2 消息类型表新增——D49⑤）。
type ProgressMessage struct {
	Type     string         `json:"type"`      // "progress"
	ActionID string         `json:"action_id"` // Action ID
	Progress ProgressDetail `json:"progress"`  // 进度详情
}

// ProgressDetail — 进度详情（completed_tokens/total_tokens_estimate/phase/message?）。
type ProgressDetail struct {
	CompletedTokens     int    `json:"completed_tokens"`      // 已完成 token 数
	TotalTokensEstimate int    `json:"total_tokens_estimate"` // 总 token 估算
	Phase               string `json:"phase"`                 // 阶段标识
	Message             string `json:"message,omitempty"`     // 可选消息
}

// ProgressIntervalTokens — 发送频率配置读取（policy.progress_interval_tokens 透传——R-1184）。
// R-1471（发现 32）：配置驱动非常量硬编码——从 PolicyConfig 读取（默认 500）。
// 契约：每 500 token 或每 5s 取先到（policy.progress_interval_tokens 默认 500）。
func ProgressIntervalTokens(cfg interface{ GetProgressIntervalTokens() int }) int {
	return cfg.GetProgressIntervalTokens() // 默认 500（PolicyConfig 透传）
}
