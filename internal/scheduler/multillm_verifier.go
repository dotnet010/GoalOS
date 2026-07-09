// Package scheduler — Multi-LLM Verifier v0.2.0。
// 真正的多模型验证：N 个 Provider 并行独立审查 → VerdictCombiner 合并裁决。
// R-858: ColdVerify 冷验证模式。R-860: Debate 辩论轮次。
//
// 设计依据：05 架构文档 §3.3、R324、会议 #158-#159。

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

// MultiLLMVerifier 并行调用 N 个 LLM Provider 进行代码审查（v0.2.0）。
type MultiLLMVerifier struct {
	providers   []ProviderClient
	combiner    *VerdictCombiner
	coldReview  bool // R-858: 冷验证模式——只传产出物代码
	debateRound bool // R-860: 辩论轮次——Round 1 分歧时触发 Round 2
}

// ProviderClient 封装单个 Provider 的 LLM 客户端。
type ProviderClient struct {
	Name   string
	Model  string
	Client *missionengine.CloudLLMClient
}

// NewMultiLLMVerifier 创建多模型验证器。
func NewMultiLLMVerifier(providers []ProviderClient) *MultiLLMVerifier {
	return &MultiLLMVerifier{
		providers:   providers,
		combiner:    NewVerdictCombiner(),
		coldReview:  false, // R-858: 默认关闭
		debateRound: false, // R-860: 默认关闭
	}
}

// SetColdReview 启用/禁用冷验证模式（R-858）。
func (mv *MultiLLMVerifier) SetColdReview(enabled bool) { mv.coldReview = enabled }

// SetDebateRound 启用/禁用辩论轮次（R-860）。
func (mv *MultiLLMVerifier) SetDebateRound(enabled bool) { mv.debateRound = enabled }

// ColdVerify 冷验证——只传纯产出物代码，不含项目上下文（R-858）。
// 与 Codex Cold Validation 对齐：验证者不可见 builder 的 Goal 文本、Plan、workspace 上下文。
func (mv *MultiLLMVerifier) ColdVerify(artifactCode string, actionID string) (*Verdict, error) {
	if len(mv.providers) == 0 {
		return &Verdict{Result: "WARN", Consensus: true}, nil
	}

	var wg sync.WaitGroup
	votes := make([]ProviderVote, len(mv.providers))

	for i, p := range mv.providers {
		wg.Add(1)
		go func(idx int, provider ProviderClient) {
			defer wg.Done()
			votes[idx] = mv.callProviderCold(provider, artifactCode, actionID)
		}(i, p)
	}

	wg.Wait()

	validVotes := make([]ProviderVote, 0, len(votes))
	for _, v := range votes {
		if v.Vote != "" {
			validVotes = append(validVotes, v)
		}
	}

	verdict := mv.combiner.Combine(validVotes)
	verdict = mv.combiner.ResolveDivergent(verdict)

	// R-860: 辩论轮次——WARN + 分歧 → Round 2
	if mv.debateRound && verdict.Result == "WARN" && verdict.Divergent {
		log.Printf("[MultiLLM] ColdVerify debate round triggered for %s", actionID)
		verdict = mv.combiner.Debate(validVotes)
	}

	log.Printf("[MultiLLM] ColdVerify action=%s verdict=%s providers=%d/%d (cold=%v debate=%v)",
		actionID, verdict.Result, len(validVotes), len(mv.providers), mv.coldReview, mv.debateRound)

	return verdict, nil
}

// callProviderCold 冷验证调用（R-862 prompt 升级——会议 #161-#162 竞品分析）。
func (mv *MultiLLMVerifier) callProviderCold(p ProviderClient, code string, actionID string) ProviderVote {
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	// R-862: 升级后的审查 prompt——基于 Codex/Claude Code/OpenClaw 最佳实践
	// ①独立性声明 ②置信度 ③结构化输出 ④严重性分层 ⑤优先级指导 ⑥用户导向
	systemPrompt := `你是 GoalOS 的独立代码审查者。你审查的代码是由另一个 AI 模型生成的——你不是在审查自己的工作。你的审查结果将直接呈现给用户，帮助他们做出是否接受这段代码的决定。

审查要求：
1. 先给出整体判定（PASS/WARN/FAIL）
2. 给出置信度（0.0-1.0。1.0=非常确定，0.5=不太确定）
3. 对每个发现的问题：指出严重程度（🔴严重 🟡警告 🔵建议）、具体问题描述、修复建议
4. 优先报告严重问题（安全漏洞、逻辑错误）。不要报告代码风格问题（除非它影响正确性）
5. 如果置信度 < 0.7，明确说明你不确定的点

输出格式：
VERDICT: PASS|WARN|FAIL
CONFIDENCE: 0.XX
FINDINGS:
- [严重程度] 问题描述 → 修复建议
SUMMARY: 一句话总结`

	prompt := fmt.Sprintf(`审查以下代码：

%s`, truncateForReview(code, 6000))

	req := &llm.ChatRequest{
		Messages: []llm.Message{
			{Role: "system", Content: systemPrompt},
			{Role: "user", Content: prompt},
		},
		MaxTokens:  800, // R-862: 增加 token 以容纳结构化输出（原 500）
		ToolChoice: "none",
	}

	resp, err := p.Client.Chat(ctx, req)
	if err != nil {
		log.Printf("[MultiLLM] ColdVerify %s/%s error: %v", p.Name, p.Model, err)
		return ProviderVote{Provider: p.Name, Model: p.Model, Vote: ""}
	}

	vote := parseVote(resp.Content)
	return ProviderVote{
		Provider:  p.Name,
		Model:     p.Model,
		Vote:      vote,
		Reasoning: resp.Content, // R-862: 保留完整结构化输出（含 CONFIDENCE + FINDINGS + SUMMARY）
	}
}

// Verify 并行调用所有 Provider 审查代码，返回合并裁决（v0.1.0，保留向后兼容）。
func (mv *MultiLLMVerifier) Verify(code string, actionID string) (*Verdict, error) {
	if len(mv.providers) == 0 {
		return &Verdict{Result: "WARN", Consensus: true}, nil
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

	validVotes := make([]ProviderVote, 0, len(votes))
	for _, v := range votes {
		if v.Vote != "" {
			validVotes = append(validVotes, v)
		}
	}

	verdict := mv.combiner.Combine(validVotes)
	verdict = mv.combiner.ResolveDivergent(verdict)

	// R-860: 辩论轮次
	if mv.debateRound && verdict.Result == "WARN" && verdict.Divergent {
		log.Printf("[MultiLLM] Verify debate round triggered for %s", actionID)
		verdict = mv.combiner.Debate(validVotes)
	}

	// R-846+R-865: 生成 ReviewReport（含诚实标注）
	report := GenerateReviewReport("", actionID, verdict)
	log.Printf("[MultiLLM] action=%s verdict=%s score=%.2f providers=%d/%d disclosure=%s",
		actionID, verdict.Result, verdict.WeightedScore, len(validVotes), len(mv.providers),
		report.HonestDisclosure[:30])

	for _, v := range validVotes {
		log.Printf("[MultiLLM]   %s/%s → %s: %.120s", v.Provider, v.Model, v.Vote, v.Reasoning)
	}

	return verdict, nil
}

// callProvider 调用单个 Provider 审查代码（R-862 prompt 升级——会议 #161-#162）。
func (mv *MultiLLMVerifier) callProvider(p ProviderClient, code string, actionID string) ProviderVote {
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	// R-862: 升级后的审查 prompt——与 callProviderCold 一致
	systemPrompt := `你是 GoalOS 的独立代码审查者。你审查的代码是由另一个 AI 模型生成的——你不是在审查自己的工作。你的审查结果将直接呈现给用户，帮助他们做出是否接受这段代码的决定。

审查要求：
1. 先给出整体判定（PASS/WARN/FAIL）
2. 给出置信度（0.0-1.0。1.0=非常确定，0.5=不太确定）
3. 对每个发现的问题：指出严重程度（🔴严重 🟡警告 🔵建议）、具体问题描述、修复建议
4. 优先报告严重问题（安全漏洞、逻辑错误）。不要报告代码风格问题（除非它影响正确性）
5. 如果置信度 < 0.7，明确说明你不确定的点

输出格式：
VERDICT: PASS|WARN|FAIL
CONFIDENCE: 0.XX
FINDINGS:
- [严重程度] 问题描述 → 修复建议
SUMMARY: 一句话总结`

	prompt := fmt.Sprintf(`审查以下代码：

%s`, truncateForReview(code, 6000))

	req := &llm.ChatRequest{
		Messages: []llm.Message{
			{Role: "system", Content: systemPrompt},
			{Role: "user", Content: prompt},
		},
		MaxTokens:  800,
		ToolChoice: "none",
	}

	resp, err := p.Client.Chat(ctx, req)
	if err != nil {
		log.Printf("[MultiLLM] %s/%s timeout/error: %v", p.Name, p.Model, err)
		return ProviderVote{Provider: p.Name, Model: p.Model, Vote: ""}
	}

	vote := parseVote(resp.Content)
	return ProviderVote{
		Provider:  p.Name,
		Model:     p.Model,
		Vote:      vote,
		Reasoning: resp.Content, // R-862: 保留完整结构化输出
	}
}

// parseVote 从 LLM 响应中提取 PASS/WARN/FAIL。
func parseVote(content string) string {
	upper := strings.ToUpper(strings.TrimSpace(content))
	for _, v := range []string{"FAIL", "WARN", "PASS"} {
		if strings.Contains(upper, v) {
			return v
		}
	}
	return "WARN"
}

func truncateForReview(code string, maxLen int) string {
	if len(code) <= maxLen {
		return code
	}
	return code[:maxLen] + "\n... (truncated)"
}
