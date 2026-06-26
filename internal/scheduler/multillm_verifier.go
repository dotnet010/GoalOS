// Package scheduler — Multi-LLM Verifier v1.1.0。
// 真正的多模型验证：N 个 Provider 并行独立审查 → VerdictCombiner 合并裁决。
//
// 设计依据：05 架构文档 §3.3、R324。

package scheduler

import (
	"context"
	"fmt"
	"strings"
	"log"
	"sync"
	"time"

	"github.com/goalos/goalos/internal/llm"
	"github.com/goalos/goalos/internal/missionengine"
)

// MultiLLMVerifier 并行调用 N 个 LLM Provider 进行代码审查（v1.1.0）。
type MultiLLMVerifier struct {
	providers []ProviderClient
	combiner  *VerdictCombiner
}

// ProviderClient 封装单个 Provider 的 LLM 客户端。
type ProviderClient struct {
	Name    string
	Model   string
	Client  *missionengine.CloudLLMClient
}

// NewMultiLLMVerifier 创建多模型验证器。
func NewMultiLLMVerifier(providers []ProviderClient) *MultiLLMVerifier {
	return &MultiLLMVerifier{
		providers: providers,
		combiner:  NewVerdictCombiner(),
	}
}

// Verify 并行调用所有 Provider 审查代码，返回合并裁决（v1.1.0）。
func (mv *MultiLLMVerifier) Verify(code string, actionID string) (*Verdict, error) {
	if len(mv.providers) == 0 {
		return &Verdict{Result: "PASS", Consensus: true}, nil
	}

	var wg sync.WaitGroup
	votes := make([]ProviderVote, len(mv.providers))

	for i, p := range mv.providers {
		wg.Add(1)
		go func(idx int, provider ProviderClient) {
			defer wg.Done()
			votes[idx] = mv.callProvider(provider, code, actionID)
		}(i, p)
	}

	wg.Wait()

	// 过滤超时/失败的投票
	validVotes := make([]ProviderVote, 0, len(votes))
	for _, v := range votes {
		if v.Vote != "" {
			validVotes = append(validVotes, v)
		}
	}

	verdict := mv.combiner.Combine(validVotes)
	log.Printf("[MultiLLM] action=%s verdict=%s score=%.2f providers=%d/%d",
		actionID, verdict.Result, verdict.WeightedScore, len(validVotes), len(mv.providers))

	return verdict, nil
}

// callProvider 调用单个 Provider 审查代码（30s 超时）。
func (mv *MultiLLMVerifier) callProvider(p ProviderClient, code string, actionID string) ProviderVote {
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	prompt := fmt.Sprintf(`审查以下代码。返回 PASS（没有问题）/ WARN（有小问题但不阻塞）/ FAIL（有严重问题必须修改）。

代码:
%s

请只返回一个词: PASS, WARN, 或 FAIL。`, truncateForReview(code, 8000))

	req := &llm.ChatRequest{
		Messages: []llm.Message{
			{Role: "system", Content: "你是代码审查专家。严格审查安全、性能、正确性。只返回一个词: PASS, WARN, 或 FAIL。"},
			{Role: "user", Content: prompt},
		},
		MaxTokens:  10,
		ToolChoice: "none", // 审查模式不需要 function calling
	}

	resp, err := p.Client.Chat(ctx, req)
	if err != nil {
		log.Printf("[MultiLLM] %s/%s timeout/error: %v", p.Name, p.Model, err)
		return ProviderVote{Provider: p.Name, Model: p.Model, Vote: ""} // ABSTAIN
	}

	vote := parseVote(resp.Content)
	return ProviderVote{
		Provider:  p.Name,
		Model:     p.Model,
		Vote:      vote,
		Reasoning: resp.Content,
	}
}

// parseVote 从 LLM 响应中提取 PASS/WARN/FAIL。
func parseVote(content string) string {
	upper := strings.ToUpper(strings.TrimSpace(content))
	for _, v := range []string{"FAIL", "WARN", "PASS"} { // 严格优先
		if strings.Contains(upper, v) { return v }
	}
	return "WARN"
}

func truncateForReview(code string, maxLen int) string {
	if len(code) <= maxLen { return code }
	return code[:maxLen] + "\n... (truncated)"
}
