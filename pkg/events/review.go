// Package events — ReviewReport 数据模型 v0.2.0（R-846 — 会议 #156）
// MultiLLM 验证完成后 VerdictCombiner 生成的结构化审查报告。
// 持久化至 ~/.goalos/events/<goal_id>/reviews/<action_id>.json。
// 文件权限 0600（Kees Cook 裁定——安全敏感度高于 events.jsonl 的 0644）。
package events

// ReviewReport 是 MultiLLM 验证的完整审查报告。
// 生成时机：VerdictCombiner 执行投票裁决（R-844）后立即生成并写入。
// Invariant（Meyer）: Provider 原始意见完整保留——不截断、不翻译、不美化。
type ReviewReport struct {
	GoalID            string           `json:"goal_id"`
	ActionID          string           `json:"action_id"`
	Verdict           string           `json:"verdict"`            // "PASS" | "WARN" | "FAIL"（R-844 投票制结果）
	VoteDistribution  VoteDist         `json:"vote_distribution"`  // 投票分布摘要
	ProviderOpinions  []ProviderOpinion `json:"provider_opinions"` // 每个 Provider 的独立审查意见
	SemanticMetaVerdict *string        `json:"semantic_meta_verdict,omitempty"` // 语义元验证结果（如有）
	UserDecision       *UserDecision   `json:"user_decision"`                   // 用户的最终决定（null = 未决策）
	CreatedAt          string          `json:"created_at"`                      // ISO 8601
	HonestDisclosure   string          `json:"honest_disclosure"`               // R-865: 诚实标注——AI审查是概率性判断
}

// HonestDisclosureText 是 R-865 定义的诚实标注文本。
// 显示在审查面板顶部，告知用户 AI 审查的性质和局限。
const HonestDisclosureText = "AI 审查是概率性判断，不替代确定性验证。多个 AI 模型的一致意见提高了置信度，但不能消除共享盲区。最终裁决：自动化测试 + 行为测试。"

// VoteDist 是投票分布摘要（R-844）。
// R-1331/F-21：Abstain 字段已删除——S-20 投票值枚举无弃权位，
// 超时在采集层记 FAIL(reason=llm_timeout)，不存在 ABSTAIN 值。
type VoteDist struct {
	Pass int `json:"pass"`
	Warn int `json:"warn"`
	Fail int `json:"fail"`
}

// ProviderOpinion 是单个 AI Provider 的审查意见。
// Invariant（Meyer R-846）: Reasoning 字段完整保留——不截断（折叠是 UI 行为，非数据丢失）、
// 不翻译（翻译 = 改变语义）、不美化（原始文本，非 Markdown 转换）。
type ProviderOpinion struct {
	Provider   string `json:"provider"`    // "anthropic" | "openai" | "ollama"
	Model      string `json:"model"`       // "claude-sonnet-4-6" | "gpt-4o" | "llama3.1"
	Vote       string `json:"vote"`        // "PASS" | "WARN" | "FAIL"
	Reasoning  string `json:"reasoning"`   // [MUST] 完整原始推理。R-853 sanitization 已执行。
	DurationMs int    `json:"duration_ms"` // Provider 响应耗时
}

// UserDecision 是用户对 MultiLLM 审查结果的决策（R-850）。
// Decision 取值：accept（接受结果，tainted_review=true）|
//
//	retry（新 Session 重做，携带 feedback）|
//	refine（修改需求→Agent.Replan）
//
// Invariant（Meyer R-850）: 决策不可撤销。同一 Action 最多发布一次 MultiLLMUserDecided。
type UserDecision struct {
	Decision  string `json:"decision"`             // "accept" | "retry" | "refine"
	DecidedAt string `json:"decided_at"`           // ISO 8601
	Feedback  string `json:"feedback,omitempty"`   // retry 或 refine 时的用户反馈
	Tainted   bool   `json:"tainted"`              // decision=accept → true
}

// SanitizeReasoning 执行写入前 sanitization（R-853 MUST）。
// 删除 reasoning 中匹配 API Key 模式的内容。
// 规则：sk-or-[a-zA-Z0-9]+, sk-ant-[a-zA-Z0-9]+, ghp_[a-zA-Z0-9]+, xox[baprs]-[a-zA-Z0-9]+
func SanitizeReasoning(reasoning string) string {
	// 使用正则删除 API Key 模式。
	// 实现细节见 review_test.go TC-MLR-006。
	s := reasoning
	// 删除匹配的模式——替换为 "[REDACTED]"
	patterns := []string{
		"sk-or-", "sk-ant-", "ghp_", "xoxb-", "xoxa-", "xoxp-", "xoxr-", "xoxs-",
	}
	for _, prefix := range patterns {
		for {
			idx := findPatternStart(s, prefix)
			if idx < 0 {
				break
			}
			end := findPatternEnd(s, idx)
			s = s[:idx] + "[REDACTED]" + s[end:]
		}
	}
	return s
}

// findPatternStart 在 s 中查找以 prefix 开头的 API Key 模式起始位置。
func findPatternStart(s, prefix string) int {
	for i := 0; i <= len(s)-len(prefix); i++ {
		if s[i:i+len(prefix)] == prefix {
			return i
		}
	}
	return -1
}

// findPatternEnd 找到 API Key 模式的结束位置（下一个空白字符或字符串末尾）。
func findPatternEnd(s string, start int) int {
	for i := start; i < len(s); i++ {
		if s[i] == ' ' || s[i] == '\n' || s[i] == '\t' || s[i] == ',' || s[i] == '"' || s[i] == '\'' {
			return i
		}
	}
	return len(s)
}
