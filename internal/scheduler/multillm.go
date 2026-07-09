// Package scheduler — Multi-LLM VerdictCombiner v1.1.0。
// 两阶段裁决：快速加权 + 语义元验证。ModelRouter 数据分级路由。
//
// 设计依据：05 架构文档 §3.3、R324。

package scheduler

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"strings"
	"time"

	"github.com/goalos/goalos/pkg/events"
)

// ProviderVote 是单个 LLM Provider 的验证投票。
type ProviderVote struct {
	Provider  string `json:"provider"`   // "anthropic"|"openai"|"ollama"
	Model     string `json:"model"`      // "claude-sonnet-4-6"
	Vote      string `json:"vote"`       // "PASS"|"WARN"|"FAIL"
	Reasoning string `json:"reasoning"`  // 判断依据
}

// VerdictCombiner 是 Multi-LLM 裁决器（v1.1.0 两阶段）。
type VerdictCombiner struct {
	providerReliability map[string]float64 // provider → 可靠性权重（1.0=全权重）
}

// NewVerdictCombiner 创建裁决器。
func NewVerdictCombiner() *VerdictCombiner {
	return &VerdictCombiner{
		providerReliability: make(map[string]float64),
	}
}

// Verdict 是最终裁决结果。
type Verdict struct {
	Result        string         `json:"result"`       // "PASS"|"WARN"|"FAIL"
	WeightedScore float64        `json:"weighted_score"`
	Votes         []ProviderVote `json:"votes"`
	Consensus     bool           `json:"consensus"`
	NeedsMeta     bool           `json:"needs_meta_verification"` // 是否需要语义元验证
	Divergent     bool           `json:"divergent"`               // 是否存在实质性分歧
	DebatePrompt  string         `json:"debate_prompt,omitempty"` // R-860: 辩论轮次 prompt
}

// Combine 执行两阶段裁决（v1.1.0）。
// 阶段 1: 快速加权。阶段 2: 语义元验证（实质性分歧时标记 needs_meta=true）。
func (vc *VerdictCombiner) Combine(votes []ProviderVote) *Verdict {
	if len(votes) == 0 {
		return &Verdict{Result: "WARN", Consensus: true}
	}

	v := &Verdict{Votes: votes}

	// 阶段 1: 快速加权
	v.WeightedScore = vc.weightedScore(votes)
	v.Consensus = vc.isConsensus(votes)
	v.Divergent = vc.isDivergent(votes)

	switch {
	case v.WeightedScore > 1.5:
		v.Result = "FAIL"
	case v.WeightedScore > 0.8:
		v.Result = "WARN"
		// 实质性分歧→需要语义元验证
		if v.Divergent {
			v.NeedsMeta = true
		}
	default:
		v.Result = "PASS"
	}

	return v
}

// weightedScore 计算加权分数。
// FAIL 权重 3, WARN 权重 2, PASS 权重 1。应用 Provider 可靠性系数。
func (vc *VerdictCombiner) weightedScore(votes []ProviderVote) float64 {
	if len(votes) == 0 {
		return 0
	}
	var total float64
	for _, vote := range votes {
		weight := vc.voteWeight(vote.Vote)
		reliability := vc.providerReliability[vote.Provider]
		if reliability == 0 {
			reliability = 1.0 // 默认全权重
		}
		total += float64(weight) * reliability
	}
	return total / float64(len(votes))
}

// voteWeight 返回投票权重。
func (vc *VerdictCombiner) voteWeight(vote string) int {
	switch vote {
	case "FAIL":
		return 3
	case "WARN":
		return 2
	case "PASS":
		return 1
	default:
		return 1 // ABSTAIN 等同 PASS
	}
}

// isConsensus 判断是否所有投票一致。
func (vc *VerdictCombiner) isConsensus(votes []ProviderVote) bool {
	if len(votes) <= 1 {
		return true
	}
	first := votes[0].Vote
	for _, v := range votes[1:] {
		if v.Vote != first {
			return false
		}
	}
	return true
}

// isDivergent 判断是否存在实质性分歧——任意两个 Provider 投票不同。
func (vc *VerdictCombiner) isDivergent(votes []ProviderVote) bool {
	hasFail, hasWarn, hasPass := false, false, false
	for _, v := range votes {
		switch v.Vote {
		case "FAIL": hasFail = true
		case "WARN": hasWarn = true
		case "PASS": hasPass = true
		}
	}
	// R-843: 任何投票不一致→分歧
	return (hasFail && hasWarn) || (hasFail && hasPass) || (hasWarn && hasPass)
}

// ResolveDivergent 解决 Provider 投票分歧（R-830 重命名自 SemanticMetaVerify）。
// 规则: 多数 FAIL→FAIL，其他→WARN。纯本地计算，无需额外 LLM 调用。
func (vc *VerdictCombiner) ResolveDivergent(v *Verdict) *Verdict {
	// R-843: consensus → 直接取投票结果，不使用加权评分
	if v.Consensus && len(v.Votes) > 0 {
		v.Result = v.Votes[0].Vote
		v.NeedsMeta = false
		return v
	}
	if !v.NeedsMeta {
		return v
	}
	// H19: 记录分歧详情——Provider 投票分布
	failCount, warnCount, passCount := 0, 0, 0
	for _, vote := range v.Votes {
		switch vote.Vote {
		case "FAIL": failCount++
		case "WARN": warnCount++
		case "PASS": passCount++
		}
	}
	log.Printf("[VerdictCombiner] ResolveDivergent: FAIL=%d WARN=%d PASS=%d", failCount, warnCount, passCount)
	// R-843: 多数 FAIL→FAIL；全部 WARN→WARN（不是 FAIL）
	if failCount > passCount && failCount > warnCount {
		v.Result = "FAIL"
	} else if failCount > 0 {
		v.Result = "WARN"
	} else {
		v.Result = "PASS"
	}
	v.NeedsMeta = false
	return v
}

// Debate 执行辩论轮次——将 Round 1 各 Provider 的 reasoning 交叉注入，重新投票（R-860）。
// 仅当 Round 1 verdict = WARN 且存在分歧时触发。Round 2 结果覆盖 Round 1。
// 辩论轮次不递归——只执行一轮。
func (vc *VerdictCombiner) Debate(round1Votes []ProviderVote) *Verdict {
	if len(round1Votes) <= 1 {
		return vc.fallbackVerdict(round1Votes)
	}

	// 构建 Round 2 prompt：包含 Round 1 所有 reasoning
	var sb strings.Builder
	sb.WriteString("以下是其他 AI 模型的审查意见。请审视你的初始判断——你同意吗？如果不同意，请解释为什么。\n\n")
	for i, v := range round1Votes {
		sb.WriteString(fmt.Sprintf("模型 %d (%s/%s): %s — %s\n", i+1, v.Provider, v.Model, v.Vote, v.Reasoning))
	}
	sb.WriteString("\n请基于以上交叉意见重新判定。先一行判定(PASS/WARN/FAIL)，再一行理由。")

	debatePrompt := sb.String()

	// Round 2 投票——使用简化的内部投票（不重新调用 LLM）
	// 实际调用由 MultiLLMVerifier 的 callProvider 完成——此处返回 debate prompt
	v := &Verdict{
		Votes:        round1Votes,
		Consensus:    false,
		Divergent:    true,
		NeedsMeta:    false,
		DebatePrompt: debatePrompt,
	}

	// Round 2 加权评分——基于 Round 1 投票重新计算，但标记为 debate 结果
	v.WeightedScore = vc.weightedScore(round1Votes)
	switch {
	case v.WeightedScore > 1.5:
		v.Result = "FAIL"
	case v.WeightedScore > 0.8:
		v.Result = "WARN"
	default:
		v.Result = "PASS"
	}

	log.Printf("[VerdictCombiner] Debate: round1=%s → round2=%s (score=%.2f, votes=%d)",
		round1Votes[0].Vote, v.Result, v.WeightedScore, len(round1Votes))
	return v
}

// Verdict.DebatePrompt 存储辩论轮次的交叉 prompt（供 MultiLLMVerifier 使用）。
// 定义在 Verdict struct 中——此处为方法文档。

func (vc *VerdictCombiner) fallbackVerdict(votes []ProviderVote) *Verdict {
	if len(votes) == 0 {
		return &Verdict{Result: "WARN", Consensus: true}
	}
	return &Verdict{Result: votes[0].Vote, Consensus: true, Votes: votes}
}

// UpdateReliability 更新 Provider 可靠性权重。
// 孤立投票（与其他所有 Provider 不同且最终被推翻）→降权。
func (vc *VerdictCombiner) UpdateReliability(provider string, isIsolated bool) {
	if isIsolated {
		vc.providerReliability[provider] *= 0.5
		log.Printf("[VerdictCombiner] %s reliability degraded to %.2f", provider, vc.providerReliability[provider])
	} else {
		// 正常投票→缓慢恢复
		if rel, ok := vc.providerReliability[provider]; ok && rel < 1.0 {
			vc.providerReliability[provider] = rel + 0.05
		}
	}
}

// ModelRouter 是 LLM Provider 路由选择器（v1.1.0 数据分级路由）。
type ModelRouter struct {
	localFirst bool       // L0-L1 始终仅本地
	providers  []ProviderConfig
}

// ProviderConfig 是 LLM Provider 配置。
type ProviderConfig struct {
	Name       string   `json:"name"`        // "anthropic"
	Endpoint   string   `json:"endpoint"`    // "https://api.anthropic.com"
	AllowedFor []string `json:"allowed_for"` // ["L2","L3","L4","L5"]
	Model      string   `json:"model"`       // "claude-sonnet-4-6"
}

// NewModelRouter 创建 ModelRouter。
func NewModelRouter(localFirst bool, providers []ProviderConfig) *ModelRouter {
	return &ModelRouter{localFirst: localFirst, providers: providers}
}

// Route 根据风险等级返回应使用的 Provider 列表（v1.1.0 数据分级路由）。
// L0-L1: 仅本地（localFirst=true 时）。L2-L3: 本地+1云端。L4-L5: 全部允许的 Provider。
func (mr *ModelRouter) Route(riskLevel string) []ProviderConfig {
	var result []ProviderConfig

	// L0-L1: 仅本地。返回空列表+日志提示——调用者使用本地 Ollama
	if mr.localFirst && (riskLevel == "L0" || riskLevel == "L1") {
		log.Printf("[ModelRouter] risk=%s → local-only (data stays on device)", riskLevel)
		return result // 空列表——调用者应检测并 fallback 到本地 Ollama
	}

	for _, p := range mr.providers {
		if mr.isAllowed(p, riskLevel) {
			result = append(result, p)
		}
	}

	// L2-L3: 限制为最多 1 个云端 Provider
	if strings.HasPrefix(riskLevel, "L2") || riskLevel == "L3" {
		if len(result) > 1 {
			result = result[:1]
		}
	}

	return result
}

// isAllowed 检查 Provider 是否允许用于指定风险等级。
func (mr *ModelRouter) isAllowed(p ProviderConfig, riskLevel string) bool {
	for _, allowed := range p.AllowedFor {
		if allowed == riskLevel {
			return true
		}
	}
	return false
}

// ─── ReviewReport 生成（R-846 — 会议 #156）─────────────────────────────

// GenerateReviewReport 从 VerdictCombiner 的裁决结果生成 ReviewReport。
// 在 VerdictCombiner.Combine() 完成投票裁决后调用。
// 执行 sanitization（R-853）后返回。
func GenerateReviewReport(goalID, actionID string, verdict *Verdict) *events.ReviewReport {
	dist := countVotes(verdict.Votes)

	opinions := make([]events.ProviderOpinion, len(verdict.Votes))
	for i, v := range verdict.Votes {
		opinions[i] = events.ProviderOpinion{
			Provider:   v.Provider,
			Model:      v.Model,
			Vote:       v.Vote,
			Reasoning:  events.SanitizeReasoning(v.Reasoning), // R-853: sanitize API keys
			DurationMs: 0, // 由 PluginRunner 在收集结果时填充
		}
	}

	report := &events.ReviewReport{
		GoalID:           goalID,
		ActionID:         actionID,
		Verdict:          verdict.Result,
		VoteDistribution: dist,
		ProviderOpinions: opinions,
		CreatedAt:        time.Now().UTC().Format(time.RFC3339),
		HonestDisclosure: events.HonestDisclosureText, // R-865
	}

	// 语义元验证结果（如有）
	if verdict.NeedsMeta || verdict.Divergent {
		metaResult := "PASS"
		if verdict.Result == "FAIL" {
			metaResult = "FAIL"
		} else {
			metaResult = "WARN"
		}
		report.SemanticMetaVerdict = &metaResult
	}

	return report
}

// countVotes 统计投票分布（R-844 投票制）。
func countVotes(votes []ProviderVote) events.VoteDist {
	var dist events.VoteDist
	for _, v := range votes {
		switch v.Vote {
		case "PASS":
			dist.Pass++
		case "WARN":
			dist.Warn++
		case "FAIL":
			dist.Fail++
		default:
			dist.Abstain++
		}
	}
	return dist
}

// SummaryMessage 返回 MultiLLM 审查的人类可读摘要消息（R-847）。
func SummaryMessage(report *events.ReviewReport) string {
	total := report.VoteDistribution.Pass + report.VoteDistribution.Warn +
		report.VoteDistribution.Fail + report.VoteDistribution.Abstain
	switch report.Verdict {
	case "FAIL":
		return fmt.Sprintf("MultiLLM 审查发现 %d 个问题。%d 个模型审查，%d FAIL / %d WARN / %d PASS。",
			report.VoteDistribution.Fail, total, report.VoteDistribution.Fail, report.VoteDistribution.Warn, report.VoteDistribution.Pass)
	case "WARN":
		return fmt.Sprintf("MultiLLM 审查存在警告。%d 个模型审查，%d WARN / %d PASS。",
			total, report.VoteDistribution.Warn, report.VoteDistribution.Pass)
	default:
		return fmt.Sprintf("MultiLLM 审查通过。%d 个模型审查，全部 PASS。", total)
	}
}

// ─── Provider 健康检查（R-861 — 会议 #158）─────────────────────────────

// ProviderStatus 表示 Provider 健康检查结果。
type ProviderStatus struct {
	Name    string `json:"name"`
	Model   string `json:"model"`
	Healthy bool   `json:"healthy"`
	Code    int    `json:"code,omitempty"`    // HTTP 状态码（0=未尝试/网络错误）
	Message string `json:"message,omitempty"` // 人类可读状态描述
}

// CheckProviderHealth 测试单个 MultiLLM Provider 的连通性（R-861）。
// 发送简单 API 调用（max_tokens=1）并检查响应。HTTP 200→健康。HTTP 429→限流但仍可用。HTTP 400/5xx→不健康。
func CheckProviderHealth(cfg ProviderConfig, apiKey string) ProviderStatus {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	body := fmt.Sprintf(`{"model":"%s","messages":[{"role":"user","content":"OK"}],"max_tokens":1}`, cfg.Model)
	req, err := http.NewRequestWithContext(ctx, "POST", cfg.Endpoint+"/chat/completions", strings.NewReader(body))
	if err != nil {
		return ProviderStatus{Name: cfg.Name, Model: cfg.Model, Healthy: false, Message: err.Error()}
	}
	req.Header.Set("Authorization", "Bearer "+apiKey)
	req.Header.Set("Content-Type", "application/json")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return ProviderStatus{Name: cfg.Name, Model: cfg.Model, Healthy: false, Message: err.Error()}
	}
	defer resp.Body.Close()

	switch {
	case resp.StatusCode == 200:
		return ProviderStatus{Name: cfg.Name, Model: cfg.Model, Healthy: true, Code: 200, Message: "OK"}
	case resp.StatusCode == 429:
		// 速率限制——Provider 可用但受限
		return ProviderStatus{Name: cfg.Name, Model: cfg.Model, Healthy: true, Code: 429, Message: "rate-limited"}
	default:
		return ProviderStatus{Name: cfg.Name, Model: cfg.Model, Healthy: false, Code: resp.StatusCode,
			Message: fmt.Sprintf("HTTP %d", resp.StatusCode)}
	}
}

// CheckAllProviders 对所有 MultiLLM Provider 执行健康检查（R-861）。
// 返回健康/不健康的 Provider 列表。不健康的 Provider 在本次 daemon session 中停用。
func CheckAllProviders(providers []ProviderConfig, apiKey string) (healthy, unhealthy []ProviderStatus) {
	for _, p := range providers {
		status := CheckProviderHealth(p, apiKey)
		if status.Healthy {
			healthy = append(healthy, status)
			log.Printf("[MultiLLM] Provider health ✅ %s/%s: %s", p.Name, p.Model, status.Message)
		} else {
			unhealthy = append(unhealthy, status)
			log.Printf("[MultiLLM] Provider health ❌ %s/%s: %s", p.Name, p.Model, status.Message)
		}
	}
	return
}
