// Package missionengine — GoalAgent LLM 驱动实现。
// 动态 system prompt + Goal + Context → MissionGraph。
// 通过 LLMClient 接口调用 LLM（any-llm-go 或兼容 Provider）。
// MVP 默认使用 StubAgent，配置 LLM Provider 后自动切换到 GoalAgent。
//
// 设计依据：05 架构文档 §5、R124、R199。
package missionengine

import (
	"encoding/json"
	"fmt"
	"log"
	"strings"
)

// GoalAgent 是 LLM 驱动的 Agent 实现。
// 使用 any-llm-go（Mozilla 官方）统一 LLM 接口。
type GoalAgent struct {
	llm LLMClient // LLM 客户端接口
}

// LLMClient 是 LLM API 调用接口。
// 底层实现：any-llm-go。Provider 通过配置切换。
type LLMClient interface {
	Chat(systemPrompt string, userMessage string) (string, error)
}

// NewGoalAgent 创建 GoalAgent。
func NewGoalAgent(llm LLMClient) *GoalAgent {
	return &GoalAgent{llm: llm}
}

// Plan 使用 LLM 将 Goal 拆解为 MissionGraph。
func (g *GoalAgent) Plan(goal string, ctx Context) (*MissionGraph, error) {
	systemPrompt := g.buildSystemPrompt(ctx)
	userMessage := g.buildUserMessage(goal)

	log.Printf("[GoalAgent] planning goal: %s", goal)
	response, err := g.llm.Chat(systemPrompt, userMessage)
	if err != nil {
		return nil, fmt.Errorf("GoalAgent: LLM 调用失败: %w", err)
	}

	graph, err := g.parseResponse(response)
	if err != nil {
		log.Printf("[GoalAgent] 解析 MissionGraph 失败，使用 fallback: %v", err)
		return g.fallbackPlan(goal, ctx), nil
	}
	return graph, nil
}

// buildSystemPrompt 构建 system prompt。
// Layer 1 Immutable：GoalOS 系统指令 + 角色描述。
func (g *GoalAgent) buildSystemPrompt(ctx Context) string {
	prompt := `你是 GoalOS 的智能规划引擎。你的任务是将用户的目标拆解为可执行的任务图（MissionGraph）。

## 输出格式
你必须输出合法的 JSON：
{
  "nodes": [
    {"id": "1", "type": "mission", "description": "任务描述"},
    {"id": "2", "type": "mission", "description": "任务描述"}
  ],
  "edges": [
    {"from": "1", "to": "2", "type": "sequential"}
  ]
}

## 规则
- 每个 node 的 type 必须是 "mission"
- 每个 edge 的 type 必须是 "sequential" 或 "parallel"
- 任务描述使用中文。简洁清晰
- 3-7 个节点为佳。不要过于细碎
- 不产出可执行代码——只做规划`

	if ctx.AnchorCheck {
		prompt += "\n\n## GoalAnchor 检查\n请对照原始目标检查当前路径是否偏离。如果需要纠正——在 nodes 的第一个节点中注明纠正措施。"
	}
	return prompt
}

// buildUserMessage 构建用户消息。
func (g *GoalAgent) buildUserMessage(goal string) string {
	return fmt.Sprintf("目标：%s\n请拆解为可执行的任务图。", goal)
}

// parseResponse 从 LLM 响应中解析 MissionGraph JSON。
func (g *GoalAgent) parseResponse(response string) (*MissionGraph, error) {
	// 提取 JSON 块（LLM 可能在 JSON 前后附加文字）
	start := strings.Index(response, "{")
	end := strings.LastIndex(response, "}")
	if start == -1 || end == -1 || start >= end {
		return nil, fmt.Errorf("响应中未找到 JSON")
	}
	jsonStr := response[start : end+1]

	var parsed struct {
		Nodes []struct {
			ID          string `json:"id"`
			Type        string `json:"type"`
			Description string `json:"description"`
		} `json:"nodes"`
		Edges []struct {
			From string `json:"from"`
			To   string `json:"to"`
			Type string `json:"type"`
		} `json:"edges"`
	}
	if err := json.Unmarshal([]byte(jsonStr), &parsed); err != nil {
		return nil, fmt.Errorf("JSON 解析失败: %w", err)
	}
	if len(parsed.Nodes) == 0 {
		return nil, fmt.Errorf("MissionGraph 无节点")
	}

	nodes := make([]GraphNode, len(parsed.Nodes))
	for i, n := range parsed.Nodes {
		nodes[i] = GraphNode{ID: n.ID, Type: n.Type, Description: n.Description}
	}
	edges := make([]GraphEdge, len(parsed.Edges))
	for i, e := range parsed.Edges {
		edges[i] = GraphEdge{From: e.From, To: e.To, Type: e.Type}
	}
	return &MissionGraph{Nodes: nodes, Edges: edges}, nil
}

// fallbackPlan 当 LLM 解析失败时的回退计划。
func (g *GoalAgent) fallbackPlan(goal string, ctx Context) *MissionGraph {
	return &MissionGraph{
		GoalID: ctx.GoalID,
		Nodes: []GraphNode{
			{ID: "1", Type: "mission", Description: "分析需求: " + goal},
			{ID: "2", Type: "mission", Description: "设计方案"},
			{ID: "3", Type: "mission", Description: "执行实现"},
		},
		Edges: []GraphEdge{
			{From: "1", To: "2", Type: "sequential"},
			{From: "2", To: "3", Type: "sequential"},
		},
	}
}
