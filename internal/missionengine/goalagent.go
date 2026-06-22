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
	log.Printf("[GoalAgent] LLM response (%d chars): %.200s", len(response), response)

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
	prompt := `Output ONLY this exact JSON, no other text:
{"nodes":[{"id":"1","type":"mission","description":"task","action_type":"shell.execute","target":"shell command"}],"edges":[]}

action_type MUST be one of: shell.execute web.search fs.read fs.write
For creating files: action_type=shell.execute, target=the complete cat command
Example: {"id":"1","type":"mission","description":"create file","action_type":"shell.execute","target":"cat > output.html << 'EOF'\ncontent\nEOF"}
Output ONLY the JSON. No markdown. No explanation.`

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
	// 2. 提取第一个完整 JSON 对象（从 { 到匹配的 }）
	start := strings.Index(s, "{")
	if start == -1 { return "" }
	depth := 0
	end := -1
	for i := start; i < len(s); i++ {
		if s[i] == '{' { depth++ }
		if s[i] == '}' { depth--; if depth == 0 { end = i; break } }
	}
	if end == -1 { return "" }
	s = s[start : end+1]
	// 3. 去掉 trailing comma（},] 前的逗号）
	s = strings.ReplaceAll(s, ",}", "}")
	s = strings.ReplaceAll(s, ",]", "]")
	// 3b. 替换常见 Unicode 引号（LLM 经常输出 smart quotes）
	s = strings.ReplaceAll(s, "“", `"`) // "
	s = strings.ReplaceAll(s, "”", `"`) // "
	s = strings.ReplaceAll(s, "‘", "'") // '
	s = strings.ReplaceAll(s, "’", "'") // '
	// 3c. 去掉 BOM 和不可见控制字符
	s = strings.TrimLeft(s, "\xef\xbb\xbf") // UTF-8 BOM
	s = strings.Map(func(r rune) rune {
		if r < 32 && r != '\n' && r != '\t' && r != '\r' { return -1 }
		return r
	}, s)
	// 4. 修复 JSON 字符串中的未转义换行（LLM 常见错误）
	s = fixUnescapedNewlines(s)
	// 5. 去掉 // 注释行
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

// fixUnescapedNewlines 修复 JSON 字符串值中的未转义换行。
func fixUnescapedNewlines(s string) string {
	var result strings.Builder
	inString := false
	for i := 0; i < len(s); i++ {
		ch := s[i]
		if ch == '"' && (i == 0 || s[i-1] != '\\') {
			inString = !inString
		}
		if inString && ch == '\n' {
			result.WriteString("\\n")
		} else {
			result.WriteByte(ch)
		}
	}
	return result.String()
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
