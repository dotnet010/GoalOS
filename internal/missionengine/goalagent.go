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
	prompt := `你是 GoalOS 的智能规划引擎。将用户目标拆解为可执行任务图。

## 输出格式
合法 JSON：
{
  "nodes": [
    {"id": "1", "type": "mission", "description": "描述", "action_type": "shell.execute", "target": "要执行的命令"}
  ],
  "edges": [{"from": "1", "to": "2", "type": "sequential"}]
}

## action_type 选项
- "shell.execute": 运行 shell 命令（创建文件/安装依赖/执行代码）
- "web.search": 搜索信息
- "fs.write": 写入文件

## 规则
- type="mission"（必填）
- action_type 和 target 必须填。target 是传给 Plugin 的具体指令
- 代码生成任务：用 shell.execute，target 包含完整命令
- 3-5 个节点。不要过于细碎
- 只做规划，不输出代码正文`

	if ctx.AnchorCheck {
		prompt += "\n\n## GoalAnchor 检查\n请对照原始目标检查当前路径是否偏离。如果需要纠正——在 nodes 的第一个节点中注明纠正措施。"
	}
	return prompt
}

// buildUserMessage 构建用户消息。
func (g *GoalAgent) buildUserMessage(goal string) string {
	return fmt.Sprintf("目标：%s\n请拆解为可执行的任务图。", goal)
}

// cleanLLMJSON 清理 LLM 输出的常见 JSON 格式问题。
func cleanLLMJSON(raw string) string {
	s := raw
	// 1. 去掉 markdown 代码块
	if idx := strings.Index(s, "```json"); idx != -1 {
		s = s[idx+7:]
		if end := strings.Index(s, "```"); end != -1 {
			s = s[:end]
		}
	} else if idx := strings.Index(s, "```"); idx != -1 {
		s = s[idx+3:]
		if end := strings.Index(s, "```"); end != -1 {
			s = s[:end]
		}
	}
	// 2. 提取 { 到 }
	start := strings.Index(s, "{")
	end := strings.LastIndex(s, "}")
	if start == -1 || end == -1 || start >= end {
		return ""
	}
	s = s[start : end+1]
	// 3. 去掉 trailing comma（},] 前的逗号）
	s = strings.ReplaceAll(s, ",}", "}")
	s = strings.ReplaceAll(s, ",]", "]")
	// 4. 去掉 // 注释行
	lines := strings.Split(s, "\n")
	var clean []string
	for _, line := range lines {
		if idx := strings.Index(line, "//"); idx != -1 {
			line = line[:idx]
		}
		line = strings.TrimSpace(line)
		if line != "" {
			clean = append(clean, line)
		}
	}
	return strings.Join(clean, "\n")
}

// parseResponse 从 LLM 响应中解析 MissionGraph JSON。
func (g *GoalAgent) parseResponse(response string) (*MissionGraph, error) {
	jsonStr := cleanLLMJSON(response)
	if jsonStr == "" {
		return nil, fmt.Errorf("响应中未找到 JSON")
	}

	var parsed struct {
		Nodes []struct {
			ID          interface{} `json:"id"`   // 容错：接受 string 或 number
			Type        string      `json:"type"`
			Description string      `json:"description"`
			ActionType  string      `json:"action_type"`
			Target      string      `json:"target"`
		} `json:"nodes"`
		Edges []struct {
			From interface{} `json:"from"` // 容错：接受 string 或 number
			To   interface{} `json:"to"`
			Type string      `json:"type"`
		} `json:"edges"`
	}
	// 将 ID/From/To 统一转为 string
	toString := func(v interface{}) string {
		switch val := v.(type) {
		case string: return val
		case float64: return fmt.Sprintf("%.0f", val)
		default: return fmt.Sprint(v)
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
