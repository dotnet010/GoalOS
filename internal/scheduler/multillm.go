// Package scheduler — Multi-LLM VerdictCombiner v1.1.0。
// 两阶段裁决：快速加权 + 语义元验证。ModelRouter 数据分级路由。
//
// 设计依据：05 架构文档 §3.3、R324。

package scheduler

import (
	"log"
	"strings"
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
	Result      string         `json:"result"`       // "PASS"|"WARN"|"FAIL"
	WeightedScore float64      `json:"weighted_score"`
	Votes        []ProviderVote `json:"votes"`
	Consensus    bool          `json:"consensus"`
	NeedsMeta    bool          `json:"needs_meta_verification"` // 是否需要语义元验证
	Divergent    bool          `json:"divergent"`               // 是否存在实质性分歧
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

// isDivergent 判断是否存在实质性分歧——任意 Provider FAIL 而其他 PASS。
func (vc *VerdictCombiner) isDivergent(votes []ProviderVote) bool {
	hasFail := false
	hasPass := false
	for _, v := range votes {
		if v.Vote == "FAIL" {
			hasFail = true
		}
		if v.Vote == "PASS" {
			hasPass = true
		}
	}
	return hasFail && hasPass
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
