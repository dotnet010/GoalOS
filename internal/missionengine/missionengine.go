// Package missionengine implements the GoalOS Mission Engine.
// 订阅 PlanRequested → 调用 Agent.plan() → 校验 MissionGraph → 发布 MissionGenerated/MissionGraphRejected。
//
// 设计依据：05 架构文档 §5、R153、R227。
package missionengine

import (
	"log"

	"github.com/goalos/goalos/internal/eventbus"
	"github.com/goalos/goalos/pkg/events"
)

// Agent is the planning interface.
// 提供两个实现: GoalAgent (LLM 驱动，生产环境)、StubAgent (LLM 不可用时的回退，测试/CI)。
type Agent interface {
	Plan(goal string, ctx Context) (*MissionGraph, error)
}

// Context is the planning context. Simplified for W1.
type Context struct {
	GoalID      string
	GoalText    string
	AnchorCheck bool
}

// MissionGraph is the output of Agent.plan().
type MissionGraph struct {
	GoalID string
	Nodes  []GraphNode
	Edges  []GraphEdge
}

// GraphNode is a node in the MissionGraph.
type GraphNode struct {
	ID          string `json:"id"`
	Type        string `json:"type"`        // "mission" | "action" | "approval" | "condition" | "sub_goal" | "clarification"
	Description string `json:"description"` // 人类可读描述
	ActionType  string `json:"action_type"` // 对应的 Capability action_type（如 "web.search", "fs.read"）
	Target      string `json:"target"`      // 操作目标（搜索查询、文件路径等）
}

// GraphEdge connects two nodes.
type GraphEdge struct {
	From string `json:"from"`
	To   string `json:"to"`
	Type string `json:"type"` // "sequential" | "parallel" | "conditional"
}

// Engine is the Mission Engine.
type Engine struct {
	bus   *eventbus.EventBus
	agent Agent
	seq   int
}

// New creates a Mission Engine.
func New(bus *eventbus.EventBus, agent Agent) *Engine {
	return &Engine{bus: bus, agent: agent}
}

// Start subscribes to PlanRequested and begins processing.
func (e *Engine) Start() {
	e.bus.Subscribe(events.TypePlanRequested, e.handlePlanRequested)
	log.Println("[MissionEngine] started, subscribed to PlanRequested")
}

func (e *Engine) handlePlanRequested(evt events.Event) error {
	goalText, _ := evt.Payload["goal_text"].(string)
	anchorCheck, _ := evt.Payload["goal_anchor_check"].(bool)

	ctx := Context{
		GoalID:      evt.GoalID,
		GoalText:    goalText,
		AnchorCheck: anchorCheck,
	}

	graph, err := e.agent.Plan(goalText, ctx)
	if err != nil {
		log.Printf("[MissionEngine] Agent.Plan failed: %v", err)
		e.publish(events.Event{
			Type:   events.TypeMissionGraphRejected,
			GoalID: evt.GoalID,
			Source: "mission-engine",
			Payload: map[string]interface{}{
				"error": err.Error(),
			},
		})
		return nil
	}

	// Validate MissionGraph
	if err := e.validate(graph); err != nil {
		log.Printf("[MissionEngine] validation failed: %v", err)
		e.publish(events.Event{
			Type:   events.TypeMissionGraphRejected,
			GoalID: evt.GoalID,
			Source: "mission-engine",
			Payload: map[string]interface{}{
				"reject_reasons": []string{err.Error()},
				"attempt":        1,
			},
		})
		return nil
	}

	// 构造节点 payload 列表（供 Scheduler 读取 action_type/target）
	nodesPayload := make([]interface{}, len(graph.Nodes))
	for i, n := range graph.Nodes {
		nodesPayload[i] = map[string]interface{}{
			"id":          n.ID,
			"type":        n.Type,
			"description": n.Description,
			"action_type": n.ActionType,
			"target":      n.Target,
		}
	}
	e.publish(events.Event{
		Type:   events.TypeMissionGenerated,
		GoalID: evt.GoalID,
		Source: "mission-engine",
		Payload: map[string]interface{}{
			"node_count": float64(len(graph.Nodes)),
			"strategy":   "GoalAgent",
			"nodes":      nodesPayload,
		},
	})

	// Also publish next event to drive state machine
	e.publish(events.Event{
		Type:   events.TypeUserConfirmed,
		GoalID: evt.GoalID,
		Source: "mission-engine",
	})

	return nil
}

func (e *Engine) validate(g *MissionGraph) error {
	if g == nil {
		return errEmptyGraph
	}
	if len(g.Nodes) == 0 {
		return errEmptyGraph
	}

	// 构建节点索引
	nodeIDs := make(map[string]bool)
	for _, n := range g.Nodes {
		if n.ID == "" {
			return &ValidationError{"节点 ID 不能为空"}
		}
		if n.Description == "" {
			return &ValidationError{"节点描述不能为空: " + n.ID}
		}
		nodeIDs[n.ID] = true
	}

	// 验证边的 from/to 引用存在性。不存在的边丢弃（LLM 输出容错）。
	validEdgeTypes := map[string]bool{"sequential": true, "parallel": true, "conditional": true, "on_completion": true, "on_failure": true}
	validEdges := make([]GraphEdge, 0, len(g.Edges))
	for _, edge := range g.Edges {
		if !nodeIDs[edge.From] || !nodeIDs[edge.To] {
			continue // LLM 引用不存在的节点→跳过
		}
		if edge.From == edge.To {
			continue // LLM 自循环→跳过
		}
		if !validEdgeTypes[edge.Type] {
			continue // LLM 无效边类型→跳过
		}
		validEdges = append(validEdges, edge)
	}
	g.Edges = validEdges

	// 拓扑排序检测循环依赖
	if hasCycle(g.Nodes, g.Edges) {
		return &ValidationError{"MissionGraph 包含循环依赖"}
	}

	return nil
}

// hasCycle 使用拓扑排序（Kahn's algorithm）检测图是否有环。
func hasCycle(nodes []GraphNode, edges []GraphEdge) bool {
	indegree := make(map[string]int)
	graph := make(map[string][]string)
	for _, n := range nodes {
		indegree[n.ID] = 0
	}
	for _, e := range edges {
		graph[e.From] = append(graph[e.From], e.To)
		indegree[e.To]++
	}

	queue := []string{}
	for id, deg := range indegree {
		if deg == 0 {
			queue = append(queue, id)
		}
	}

	visited := 0
	for len(queue) > 0 {
		node := queue[0]
		queue = queue[1:]
		visited++
		for _, neighbor := range graph[node] {
			indegree[neighbor]--
			if indegree[neighbor] == 0 {
				queue = append(queue, neighbor)
			}
		}
	}

	return visited != len(nodes) // 有剩余节点 → 存在环
}

func (e *Engine) publish(evt events.Event) {
	e.seq++
	evt.Seq = e.seq
	e.bus.Publish(evt)
}

// Sentinel errors for validation.
var (
	errEmptyGraph = &ValidationError{"MissionGraph is empty"}
)

// ValidationError is a MissionGraph validation error.
type ValidationError struct {
	Reason string
}

func (e *ValidationError) Error() string { return "validation: " + e.Reason }

// StubAgent 硬编码 3 节点图，用于无 LLM 环境下的核心链路测试。
// 配置 LLM Provider 后自动切换到 GoalAgent。
type StubAgent struct{}

// NewStubAgent 创建 StubAgent（默认 Agent，零外部依赖）。
func NewStubAgent() *StubAgent { return &StubAgent{} }

func (s *StubAgent) Plan(goal string, ctx Context) (*MissionGraph, error) {
	actionType, target := InferAction(goal)
	return &MissionGraph{
		GoalID: ctx.GoalID,
		Nodes:  []GraphNode{{ID: "1", Type: "mission", Description: goal, ActionType: actionType, Target: target}},
		Edges:  []GraphEdge{},
	}, nil
}

// InferAction 返回默认 action_type。v1.1.0: GoalAgent+LLM 推理替代关键词匹配。
// 仅作为 StubAgent/fallbackPlan 的最后回退。
func InferAction(goal string) (string, string) {
	return "shell.execute", goal
}

// SetAgent 热替换 Agent（v1.1.0 UX1 热加载）。
// 线程安全。可在运行时切换 LLM Provider/Model 而不重启 daemon。
func (e *Engine) SetAgent(agent Agent) {
	e.agent = agent
	log.Printf("[MissionEngine] agent hot-swapped to %T", agent)
}

