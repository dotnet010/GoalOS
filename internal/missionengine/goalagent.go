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
	"strings"
	"time"

	"github.com/goalos/goalos/internal/eventbus"
	"github.com/goalos/goalos/internal/llm"
	"github.com/goalos/goalos/pkg/events"
)

// GoalAgent 是 LLM 驱动的 Agent 实现。
// 使用 go-openai（sashabaranov）统一 LLM 接口。
type GoalAgent struct {
	llm LLMClient  // LLM 客户端接口
	bus *eventbus.EventBus // 事件总线（用于发布 TokenUsage 等事件）
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

// Plan 使用 LLM 将 Goal 拆解为 MissionGraph。
func (g *GoalAgent) Plan(goal string, ctx Context) (*MissionGraph, error) {
	planCtx, cancel := context.WithTimeout(context.Background(), 300*time.Second)
	defer cancel()

	systemPrompt := g.buildSystemPrompt(ctx)
	userMessage := g.buildUserMessage(goal)

	log.Printf("[GoalAgent] planning goal: %s", goal)

	req := &llm.ChatRequest{
		Messages: []llm.Message{
			{Role: "system", Content: systemPrompt},
			{Role: "user", Content: userMessage},
		},
		ToolChoice: "required",
	}

	response, err := g.llm.Chat(planCtx, req)
	if err != nil {
		return nil, fmt.Errorf("GoalAgent: LLM 调用失败: %w", err)
	}

	// Token 使用追踪
	if g.bus != nil && response.Usage.TotalTokens > 0 {
		g.bus.Publish(events.Event{
			Type:   "TokenUsage",
			GoalID: ctx.GoalID,
			Source: "goal-agent",
			Payload: map[string]interface{}{
				"prompt_tokens":     response.Usage.PromptTokens,
				"completion_tokens": response.Usage.CompletionTokens,
				"total_tokens":      response.Usage.TotalTokens,
				"model":             "goal-agent-model",
			},
		})
	}

	content := response.Content
	log.Printf("[GoalAgent] LLM response (%d chars): %.200s", len(content), content)

	graph, err := g.parseResponse(content)
	if err != nil {
		// R249: Fallback 可见性 — 记录原始 LLM 输出
		log.Printf("[GoalAgent] 解析 MissionGraph 失败: %v。LLM 原始输出 (%d chars): %.500s",
			err, len(content), content)
		return g.fallbackPlan(goal, ctx), nil
	}
	return graph, nil
}

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
	return fmt.Sprintf("目标：%s\n请拆解为可执行的任务图（MissionGraph），调用 plan_goal 函数返回结果。", goal)
}

// parseResponse 从 LLM 响应中解析 MissionGraph JSON。
// 接收 Function Calling 返回的 JSON arguments 字符串。
// 优先使用 llm.ParsePlanResponse（jsonschema），降级使用手写解析。
func (g *GoalAgent) parseResponse(response string) (*MissionGraph, error) {
	// 优先使用 jsonschema 解析
	planParams, err := llm.ParsePlanResponse(response)
	if err == nil {
		return g.convertPlanParams(planParams), nil
	}

	// 降级：手写 JSON 解析（兼容不支持 jsonschema.Unmarshal 的场景）
	return g.parseResponseFallback(response)
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

// fallbackPlan 当 LLM 解析失败时，使用关键词推理作为回退。
func (g *GoalAgent) fallbackPlan(goal string, ctx Context) *MissionGraph {
	actionType, target := InferAction(goal)
	return &MissionGraph{
		GoalID: ctx.GoalID,
		Nodes: []GraphNode{
			{ID: "1", Type: "mission", Description: goal, ActionType: actionType, Target: target},
		},
		Edges: []GraphEdge{},
	}
}
