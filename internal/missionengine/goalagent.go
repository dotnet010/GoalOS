// Package missionengine — GoalAgent LLM 驱动实现。
// 动态多层 system prompt + Goal + Context → MissionGraph。
// 通过 LLMClient 接口调用 LLM（go-openai 兼容 Provider）。
// MVP 默认使用 StubAgent，配置 LLM Provider 后自动切换到 GoalAgent。
//
// 设计依据：05 架构文档 §5、R124、R199、R243、R247-R249。
package missionengine

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"strings"
	"time"

	"github.com/goalos/goalos/internal/eventbus"
	"github.com/goalos/goalos/internal/llm"
	"github.com/goalos/goalos/pkg/events"
)

// Verifier 是代码验证接口（v0.1.0 R-372）。
// MultiLLMVerifier（scheduler 包）通过适配器实现此接口。
type Verifier interface {
	Verify(code string, actionID string) (verdict string, confidence int, reasoning string, err error)
}

// GoalAgent 是 LLM 驱动的 Agent 实现。
type GoalAgent struct {
	llm             LLMClient
	bus             *eventbus.EventBus
	verifier        Verifier            // v0.1.0 R-372
	lastAlignResult *alignAnalyzeResult // v0.1.1 fix: Align+Analyze 合并调用缓存
	planTimeout     time.Duration       // Plan 阶段超时。默认 600s，由配置文件 plan_timeout 设置
}

// LLMClient 是 LLM API 调用接口。
// 底层实现：go-openai（sashabaranov/go-openai）。Provider 通过配置切换。
// 设计依据：R243（接口重设计，添加 ctx 参数和结构化响应）。
type LLMClient interface {
	Chat(ctx context.Context, req *llm.ChatRequest) (*llm.ChatResponse, error)
	ChatStream(ctx context.Context, req *llm.ChatRequest) (<-chan llm.ChatStreamEvent, error)
}

// NewGoalAgent 创建 GoalAgent。
func NewGoalAgent(llmClient LLMClient) *GoalAgent {
	return &GoalAgent{llm: llmClient}
}

// NewGoalAgentWithBus 创建带事件总线的 GoalAgent（支持 Token 追踪）。
func NewGoalAgentWithBus(llmClient LLMClient, bus *eventbus.EventBus) *GoalAgent {
	return &GoalAgent{llm: llmClient, bus: bus}
}

// SetVerifier 设置代码验证器（v0.1.0 R-372）。
func (g *GoalAgent) SetVerifier(v Verifier) { g.verifier = v }

// SetPlanTimeout 设置 Plan 阶段超时时间。
// 由 main.go 从配置文件 plan_timeout 传入。0 或未设置则使用默认 600s。
func (g *GoalAgent) SetPlanTimeout(d time.Duration) {
	if d > 0 {
		g.planTimeout = d
	} else {
		g.planTimeout = 600 * time.Second
	}
}

// getPlanTimeout 返回 Plan 阶段超时，默认 600s。
func (g *GoalAgent) getPlanTimeout() time.Duration {
	if g.planTimeout > 0 {
		return g.planTimeout
	}
	return 600 * time.Second
}

// sanitizeGoal 对用户输入进行基本清洗，防止 prompt 注入（v0.1.0 R-372）。
// 使用 XML 标签包裹用户输入，建立明确的指令边界。
func sanitizeGoal(goal string) string {
	// 移除常见的注入模式
	goal = strings.ReplaceAll(goal, "忽略之前的指令", "[过滤]")
	goal = strings.ReplaceAll(goal, "ignore previous instructions", "[filtered]")
	goal = strings.ReplaceAll(goal, "Ignore all previous", "[filtered]")
	goal = strings.ReplaceAll(goal, " disregard ", " [filtered] ")
	return goal
}

// Align 将用户目标转换为结构化完成标准（R-350）。
// [FIXED] 原代码：LLM 失败时调用 fallbackCriteria 返回假数据 + nil error
// [FIXED] 现在：LLM 失败即返回真实错误，绝不返回假数据
func (g *GoalAgent) Align(goal string, ctx Context) (*CompletionCriteria, error) {
	result, err := g.alignAndAnalyze(goal, ctx)
	if err != nil {
		// [FIXED] 返回真实错误，不调用任何 fallback
		return nil, fmt.Errorf("Agent.Align: %w", err)
	}
	if result == nil || result.Criteria == nil {
		// [FIXED] 防御性检查：即使 err == nil，也要验证返回值有效性
		return nil, fmt.Errorf("Agent.Align: LLM 返回了空的 criteria")
	}
	g.lastAlignResult = result // v0.1.1 fix: 缓存供 Analyze 复用
	return result.Criteria, nil
}

// Analyze 分析任务复杂度、能力需求、Flow 推荐（R-350）。
// 复用 Align 的合并调用缓存——不重复调 LLM。
// [FIXED] 原代码：缓存未命中或 LLM 失败时调用 fallbackAnalysis 返回假数据
// [FIXED] 现在：任何失败都返回真实错误
func (g *GoalAgent) Analyze(criteria *CompletionCriteria, ctx Context) (*TaskAnalysis, error) {
	if criteria == nil {
		// [FIXED] 原代码：return g.fallbackAnalysis(ctx), nil
		// [FIXED] 现在：返回错误，因为无 criteria 就无法做有意义的分析
		return nil, fmt.Errorf("Agent.Analyze: criteria 不能为空")
	}
	if g.lastAlignResult != nil {
		result := g.lastAlignResult
		g.lastAlignResult = nil
		if result.Analysis == nil {
			return nil, fmt.Errorf("Agent.Analyze: 缓存的 Analysis 为 nil")
		}
		return result.Analysis, nil
	}
	// 缓存未命中——重新调用 LLM
	result, err := g.alignAndAnalyze("", ctx)
	if err != nil {
		// [FIXED] 原代码：return g.fallbackAnalysis(ctx), nil
		// [FIXED] 现在：返回真实错误
		return nil, fmt.Errorf("Agent.Analyze: %w", err)
	}
	if result == nil || result.Analysis == nil {
		return nil, fmt.Errorf("Agent.Analyze: LLM 返回了空的 analysis")
	}
	return result.Analysis, nil
}

// alignAnalyzeResult 是 Align+Analyze 合并 LLM 调用的结果（R-350 延迟优化）。
type alignAnalyzeResult struct {
	Criteria *CompletionCriteria
	Analysis *TaskAnalysis
	cached   bool
}

// alignAndAnalyze 合并 Align+Analyze 为一次 LLM 调用。
// [FIXED] 原代码：解析失败时可能返回 fallback 假数据
// [FIXED] 现在：任何失败都返回真实错误
func (g *GoalAgent) alignAndAnalyze(goal string, ctx Context) (*alignAnalyzeResult, error) {
	planCtx, cancel := context.WithTimeout(context.Background(), g.getPlanTimeout())
	defer cancel()

	prompt := g.buildAlignAnalyzePrompt(goal, ctx)
	req := &llm.ChatRequest{
		Messages: []llm.Message{
			{Role: "system", Content: prompt},
			{Role: "user", Content: fmt.Sprintf("目标：%s\n请分析此目标并返回 JSON。", sanitizeGoal(goal))},
		},
	}

	response, err := g.llm.Chat(planCtx, req)
	if err != nil {
		// [FIXED] 直接返回错误，不调用 fallback
		return nil, fmt.Errorf("LLM 调用失败: %w", err)
	}

	g.trackTokens(ctx.GoalID, response)
	content := response.Content

	result, err := g.parseAlignAnalyzeResponse(content)
	if err != nil {
		// [FIXED] 直接返回解析错误，不调用 fallback
		return nil, fmt.Errorf("解析 LLM 响应失败: %w", err)
	}
	// 验证解析结果完整性
	if result.Criteria == nil {
		return nil, fmt.Errorf("LLM 响应缺少 criteria 字段")
	}
	if result.Analysis == nil {
		return nil, fmt.Errorf("LLM 响应缺少 analysis 字段")
	}
	result.Criteria.GoalID = ctx.GoalID
	result.Analysis.GoalID = ctx.GoalID
	return result, nil
}

// Plan 在 Flow 模板约束内生成 MissionGraph（R-350）。
// [FIXED] 原代码：LLM 失败时调用 fallbackPlan 返回假任务图
// [FIXED] 现在：LLM 失败即返回真实错误
func (g *GoalAgent) Plan(criteria *CompletionCriteria, analysis *TaskAnalysis, flowName string, ctx Context) (*MissionGraph, error) {
	goal := ctx.GoalText
	if criteria != nil && criteria.SuccessDefinition != "" {
		goal = criteria.SuccessDefinition
	}
	return g.planInternal(goal, flowName, ctx)
}

// planInternal 是 Plan 的内部实现。R-724: PlanLegacy 已删除——LLM 失败即诚实失败。
// [FIXED] 原代码：解析失败时调用 fallbackPlan 返回假数据
// [FIXED] 现在：任何失败都返回真实错误
func (g *GoalAgent) planInternal(goal string, flowName string, ctx Context) (*MissionGraph, error) {
	planCtx, cancel := context.WithTimeout(context.Background(), g.getPlanTimeout())
	defer cancel()

	systemPrompt := g.buildSystemPrompt(ctx)
	userMessage := g.buildUserMessage(goal)
	if flowName != "" {
		userMessage += fmt.Sprintf("\n使用 Flow 模板: %s。请在此模板约束内生成任务图。", flowName)
	}

	req := &llm.ChatRequest{
		Messages: []llm.Message{
			{Role: "system", Content: systemPrompt},
			{Role: "user", Content: userMessage},
		},
		ToolChoice: "required",
	}

	response, err := g.llm.Chat(planCtx, req)
	if err != nil {
		// [FIXED] 直接返回错误，不调用 fallbackPlan
		return nil, fmt.Errorf("LLM 调用失败: %w", err)
	}

	g.trackTokens(ctx.GoalID, response)
	content := response.Content
	log.Printf("[GoalAgent] LLM response (%d chars): %.200s", len(content), content)
	// 完整原始输出写入文件——用于调试
	rawFile := fmt.Sprintf("/tmp/llm-raw-%s.json", ctx.GoalID)
	os.WriteFile(rawFile, []byte(content), 0644)
	log.Printf("[GoalAgent] Full raw LLM output written to %s", rawFile)

	graph, err := g.parseResponse(content)
	if err != nil {
		log.Printf("[GoalAgent] 解析 MissionGraph 失败: %v。LLM 原始输出 (%d chars): %.500s",
			err, len(content), content)
		// [FIXED] 直接返回解析错误，不调用 fallbackPlan
		return nil, fmt.Errorf("解析 LLM 响应失败: %w。完整原始输出见 %s", err, rawFile)
	}
	// [FIXED] 验证图不为空
	if graph == nil {
		return nil, fmt.Errorf("LLM 返回了空的 MissionGraph")
	}
	if len(graph.Nodes) == 0 {
		return nil, fmt.Errorf("LLM 返回了空的节点列表")
	}
	return graph, nil
}

// trackTokens 追踪 LLM Token 使用。
func (g *GoalAgent) trackTokens(goalID string, response *llm.ChatResponse) {
	if g.bus != nil && response.Usage.TotalTokens > 0 {
		g.bus.Publish(events.Event{
			Type:   events.TypeTokenUsage,
			GoalID: goalID,
			Source: "goal-agent",
			Payload: map[string]interface{}{
				"prompt_tokens":     response.Usage.PromptTokens,
				"completion_tokens": response.Usage.CompletionTokens,
				"total_tokens":      response.Usage.TotalTokens,
				"model":             "goal-agent-model",
			},
		})
	}
}

// buildAlignAnalyzePrompt 构建 Align+Analyze 合并 prompt（R-350）。
func (g *GoalAgent) buildAlignAnalyzePrompt(goal string, ctx Context) string {
	var b strings.Builder
	b.WriteString(`你是 GoalOS 的目标理解引擎。你的职责是理解用户目标并结构化分析。

## 输出格式
返回一个 JSON 对象，包含两个字段：

criteria: {
  "goal_type": "code_generation" | "data_analysis" | "research" | "content_creation" | "automation" | "other",
  "success_definition": "用自然语言描述什么叫成功完成此目标",
  "acceptance_criteria": ["可验证的验收条件1", "条件2"],
  "constraints": ["约束条件（不能做什么）"],
  "must_have": ["必须产出物"],
  "complexity": "low" | "medium" | "high" | "extreme"
}

analysis: {
  "complexity": "low" | "medium" | "high" | "extreme",
  "required_capabilities": ["shell.execute", "fs.write", ...],
  "suggested_flow": "code-project-v1" | "data-analysis-v1" | "generic-v1",
  "risk_assessment": "L0" | "L1" | "L2" | "L3" | "L4" | "L5",
  "estimated_steps": 预估步骤数,
  "reasoning": "推荐此 Flow 的理由"
}

## 核心约束
- 只输出 JSON。不输出额外文字`)
	if ctx.AnchorCheck {
		b.WriteString("\n- GoalAnchor 激活：请对照原始目标检查是否偏离")
	}
	b.WriteString("\n")
	return b.String()
}

// parseAlignAnalyzeResponse 解析 Align+Analyze 合并响应。
func (g *GoalAgent) parseAlignAnalyzeResponse(response string) (*alignAnalyzeResult, error) {
	jsonStr := response
	if idx := strings.Index(jsonStr, "{"); idx != -1 {
		jsonStr = jsonStr[idx:]
	}
	if idx := strings.LastIndex(jsonStr, "}"); idx != -1 {
		jsonStr = jsonStr[:idx+1]
	}

	var parsed struct {
		Criteria struct {
			GoalType           string   `json:"goal_type"`
			SuccessDefinition  string   `json:"success_definition"`
			AcceptanceCriteria []string `json:"acceptance_criteria"`
			Constraints        []string `json:"constraints"`
			MustHave           []string `json:"must_have"`
			Complexity         string   `json:"complexity"`
		} `json:"criteria"`
		Analysis struct {
			Complexity           string   `json:"complexity"`
			RequiredCapabilities []string `json:"required_capabilities"`
			SuggestedFlow        string   `json:"suggested_flow"`
			RiskAssessment       string   `json:"risk_assessment"`
			EstimatedSteps       int      `json:"estimated_steps"`
			Reasoning            string   `json:"reasoning"`
		} `json:"analysis"`
	}

	if err := json.Unmarshal([]byte(jsonStr), &parsed); err != nil {
		return nil, fmt.Errorf("Align+Analyze JSON 解析失败: %w", err)
	}

	return &alignAnalyzeResult{
		Criteria: &CompletionCriteria{
			GoalType:           parsed.Criteria.GoalType,
			SuccessDefinition:  parsed.Criteria.SuccessDefinition,
			AcceptanceCriteria: parsed.Criteria.AcceptanceCriteria,
			Constraints:        parsed.Criteria.Constraints,
			MustHave:           parsed.Criteria.MustHave,
			Complexity:         parsed.Criteria.Complexity,
		},
		Analysis: &TaskAnalysis{
			Complexity:           parsed.Analysis.Complexity,
			RequiredCapabilities: parsed.Analysis.RequiredCapabilities,
			SuggestedFlow:        parsed.Analysis.SuggestedFlow,
			RiskAssessment:       parsed.Analysis.RiskAssessment,
			EstimatedSteps:       parsed.Analysis.EstimatedSteps,
			Reasoning:            parsed.Analysis.Reasoning,
		},
	}, nil
}

// [DELETED] fallbackCriteria — 已删除。原函数存在致命 bug（递归调用自身导致栈溢出），
// 且返回硬编码假数据，掩盖 LLM 失败。现在 LLM 失败直接返回错误。

// [DELETED] fallbackAnalysis — 已删除。返回硬编码假分析（generic/medium/L1），
// 掩盖真实的 Analyze 失败。现在 Analyze 失败直接返回错误。

// buildSystemPrompt 构建多层 system prompt。
// 设计依据：R248（4层提示结构，支持 Prompt Caching）。
//
// Layer 1 (Immutable): GoalOS 角色定义 + 核心约束
// Layer 2 (Goal Context): 当前 Goal 文本 + GoalAnchor 上下文
// Layer 3 (Output Spec): MissionGraph schema 说明 + 字段语义
// Layer 4 (Format): 严格的输出格式约束
func (g *GoalAgent) buildSystemPrompt(ctx Context) string {
	var b strings.Builder

	// Layer 1: Immutable — GoalOS 角色与约束
	b.WriteString(`你是 GoalOS 的任务规划引擎。你的职责是将用户目标拆解为可执行的任务图（MissionGraph）。
核心约束：
- 只生成可被机器执行的步骤。不生成模糊建议
- 每个步骤必须指定具体的 action_type 和 target
- shell.execute 操作必须使用 heredoc 语法（cat > file << 'EOF'）
- 优先使用并行边（parallel）而非顺序边——除非步骤之间有明确的数据依赖
- 如果目标模糊——在第一个节点中使用 type=clarification 请求用户澄清

`)

	// Layer 2: Goal Context
	if ctx.AnchorCheck {
		b.WriteString("## GoalAnchor 检查\n")
		b.WriteString("请对照原始目标检查当前路径是否偏离。")
		b.WriteString("如果需要纠正——在 nodes 的第一个节点中注明纠正措施。\n\n")
	}

	// Layer 3: Output Spec
	b.WriteString(`## 输出 Schema
你必须调用 plan_goal 函数，传入以下 JSON 结构：

nodes 数组 — 每个节点包含：
  - id: 字符串，节点唯一标识（如 "1", "2"）
  - type: 节点类型。取值：mission（任务）、action（动作）、approval（审批）、condition（条件）、sub_goal（子目标）、clarification（澄清）
  - description: 人类可读的任务描述
  - action_type: 执行动作。取值：shell.execute、web.search、fs.read、fs.write、browser.open、browser.click
  - target: 操作目标。shell 命令、搜索查询、文件路径或 URL

edges 数组 — 每条边包含：
  - from: 源节点 ID
  - to: 目标节点 ID
  - type: 边类型。取值：sequential（顺序）、parallel（并行）、conditional（条件）、on_completion（完成时触发）、on_failure（失败时触发）

`)

	// Layer 4: Format Constraint
	b.WriteString("## 格式要求\n")
	b.WriteString("必须调用 plan_goal 函数。不输出额外文字。确保 JSON 语法完全合法。\n")
	b.WriteString("shell 命令中的换行使用 \\n 转义。heredoc 结束标记前不要有空格。\n")

	return b.String()
}

// buildUserMessage 构建用户消息。
func (g *GoalAgent) buildUserMessage(goal string) string {
	return fmt.Sprintf("目标：%s\n请拆解为可执行的任务图（MissionGraph），调用 plan_goal 函数返回结果。", sanitizeGoal(goal))
}

// parseResponse 从 LLM 响应中解析 MissionGraph JSON。
// 接收 Function Calling 返回的 JSON arguments 字符串。
// 优先使用 llm.ParsePlanResponse（jsonschema），降级使用手写解析。
// [FIXED] 原代码：两个解析路径都失败时可能返回 fallbackPlan 的假数据
// [FIXED] 现在：两个解析路径都失败时返回错误
func (g *GoalAgent) parseResponse(response string) (*MissionGraph, error) {
	// 优先使用 jsonschema 解析
	planParams, err := llm.ParsePlanResponse(response)
	if err == nil {
		return g.convertPlanParams(planParams), nil
	}

	// 降级：手写 JSON 解析（兼容不支持 jsonschema.Unmarshal 的场景）
	graph, err := g.parseResponseFallback(response)
	if err != nil {
		// [FIXED] 两个解析路径都失败，返回错误，不调用 fallbackPlan
		return nil, fmt.Errorf("所有解析路径均失败: %w", err)
	}
	return graph, nil
}

// convertPlanParams 将 PlanGoalParams 转换为 MissionGraph。
func (g *GoalAgent) convertPlanParams(p *llm.PlanGoalParams) *MissionGraph {
	nodes := make([]GraphNode, len(p.Nodes))
	for i, n := range p.Nodes {
		nodes[i] = GraphNode{
			ID: n.ID, Type: n.Type, Description: n.Description,
			ActionType: n.ActionType, Target: n.Target,
		}
	}
	edges := make([]GraphEdge, len(p.Edges))
	for i, e := range p.Edges {
		edges[i] = GraphEdge{From: e.From, To: e.To, Type: e.Type}
	}
	return &MissionGraph{Nodes: nodes, Edges: edges}
}

// parseResponseFallback 手写 JSON 解析（降级路径）。
// 保留基础的容错逻辑：接受 string/number 混合的 id/from/to。
func (g *GoalAgent) parseResponseFallback(response string) (*MissionGraph, error) {
	jsonStr := response
	// 提取 JSON 对象
	if idx := strings.Index(jsonStr, "{"); idx != -1 {
		jsonStr = jsonStr[idx:]
	}
	if idx := strings.LastIndex(jsonStr, "}"); idx != -1 {
		jsonStr = jsonStr[:idx+1]
	}
	if jsonStr == "" {
		return nil, fmt.Errorf("响应中未找到 JSON")
	}

	var parsed struct {
		Nodes []struct {
			ID          interface{} `json:"id"`
			Type        string      `json:"type"`
			Description string      `json:"description"`
			ActionType  string      `json:"action_type"`
			Target      string      `json:"target"`
		} `json:"nodes"`
		Edges []struct {
			From interface{} `json:"from"`
			To   interface{} `json:"to"`
			Type string      `json:"type"`
		} `json:"edges"`
	}
	toString := func(v interface{}) string {
		switch val := v.(type) {
		case string:
			return val
		case float64:
			return fmt.Sprintf("%.0f", val)
		default:
			return fmt.Sprint(v)
		}
	}
	if err := json.Unmarshal([]byte(jsonStr), &parsed); err != nil {
		return nil, fmt.Errorf("JSON 解析失败: %w", err)
	}
	if len(parsed.Nodes) == 0 {
		return nil, fmt.Errorf("MissionGraph 无节点")
	}

	nodes := make([]GraphNode, len(parsed.Nodes))
	for i, n := range parsed.Nodes {
		nodes[i] = GraphNode{
			ID: toString(n.ID), Type: n.Type, Description: n.Description,
			ActionType: n.ActionType, Target: n.Target,
		}
	}
	edges := make([]GraphEdge, len(parsed.Edges))
	for i, e := range parsed.Edges {
		edges[i] = GraphEdge{From: toString(e.From), To: toString(e.To), Type: e.Type}
	}
	return &MissionGraph{Nodes: nodes, Edges: edges}, nil
}

// [DELETED] fallbackPlan — 已删除。原函数返回硬编码单节点任务图（使用 InferAction 的虚假推理），
// 且存在命令注入风险（goal 字符串直接拼接到 target）。现在 Plan 失败直接返回错误。

// Verify 验证产出代码（Verifier 角色，v0.1.0 R-372）。
// GoalAgent 委托给 MultiLLMVerifier（需通过 SetVerifier 注入）。
// [FIXED] 原代码：如果未注入 verifier，返回 PASS（虚假通过）
// [FIXED] 现在：如果未注入 verifier，返回错误（未配置验证器）
func (g *GoalAgent) Verify(code string, actionID string, ctx Context) (*VerificationResult, error) {
	if g.verifier != nil {
		verdict, confidence, reasoning, err := g.verifier.Verify(code, actionID)
		if err != nil {
			// [FIXED] 原代码：返回 WARN 但 Score=0，掩盖了 verifier 错误
			// [FIXED] 现在：返回真实错误，让调用方知道验证失败
			return nil, fmt.Errorf("verifier 执行失败: %w", err)
		}
		return &VerificationResult{
			ActionID: actionID,
			Verdict:  verdict,
			Reason:   reasoning,
			Score:    confidence,
		}, nil
	}
	// [FIXED] 原代码：未注入 verifier 时返回 PASS（"basic check"），这是虚假通过
	// [FIXED] 现在：返回错误，强制要求配置 verifier
	return nil, fmt.Errorf("未配置 verifier：请通过 SetVerifier 注入验证器")
}
