// Package missionengine implements the GoalOS Mission Engine.
// 订阅 PlanRequested → 调用 Agent.plan() → 校验 MissionGraph → 发布 MissionGenerated/MissionGraphRejected。
//
// 设计依据：05 架构文档 §5、R153、R227。
package missionengine

import (
	"fmt"
	"log"
	"strings"

	"github.com/goalos/goalos/internal/eventbus"
	"github.com/goalos/goalos/pkg/events"
)

// Agent is the planning + verification interface (v0.1.0: Planner + Verifier 双角色)。
// 提供两个实现: GoalAgent (LLM 驱动，生产环境)、StubAgent (LLM 不可用时的回退，测试/CI)。
//
// Planner 角色（R-350）:
//   - Align(goal, ctx) → CompletionCriteria  — 理解目标，定义"什么叫完成"
//   - Analyze(criteria, ctx) → TaskAnalysis — 分析任务复杂度、能力需求、Flow 推荐
//   - Plan(criteria, analysis, flow, ctx) → MissionGraph — 在 Flow 约束内生成任务图
//
// Verifier 角色（R-372 会议 #63）:
//   - Verify(code, actionID, ctx) → VerificationResult — 由 Check 原语通过 QualityGate 调用
//
// 延迟优化（R-350）: Align+Analyze 在 GoalAgent 中合并为一次 LLM 调用
type Agent interface {
	// ── Planner 角色 ──
	Align(goal string, ctx Context) (*CompletionCriteria, error)
	Analyze(criteria *CompletionCriteria, ctx Context) (*TaskAnalysis, error)
	Plan(criteria *CompletionCriteria, analysis *TaskAnalysis, flowName string, ctx Context) (*MissionGraph, error)
	PlanLegacy(goal string, ctx Context) (*MissionGraph, error)

	// ── Verifier 角色（v0.1.0 R-372）──
	// Verify 对产出代码进行验证。由 PipelineRunner Check 原语通过 QualityGate 调用。
	Verify(code string, actionID string, ctx Context) (*VerificationResult, error)
}

// VerificationResult 是 Agent.Verify() 的返回结果（v0.1.0 R-372）。
type VerificationResult struct {
	ActionID string `json:"action_id"`
	Verdict  string `json:"verdict"`  // "PASS" | "WARN" | "FAIL"
	Reason   string `json:"reason"`   // 判定理由
	Score    int    `json:"score"`    // 0-100
}

// Context is the planning context.
type Context struct {
	GoalID      string
	GoalText    string
	AnchorCheck bool
}

// CompletionCriteria defines "what does done look like" for a Goal.
// Agent.Align() 产出。CompletionContract 的技术基础（R-350）。
type CompletionCriteria struct {
	GoalID             string   `json:"goal_id"`
	GoalType           string   `json:"goal_type"`           // code_generation | data_analysis | research | content_creation | automation | other
	SuccessDefinition  string   `json:"success_definition"`  // 自然语言描述"什么叫成功"
	AcceptanceCriteria []string `json:"acceptance_criteria"` // 可验证的验收条件列表
	Constraints        []string `json:"constraints"`         // 约束条件（"不能修改已有数据库"等）
	MustHave           []string `json:"must_have"`           // 必须产出物
	Complexity         string   `json:"complexity"`          // low | medium | high | extreme
}

// TaskAnalysis is the output of Agent.Analyze()（R-350）。
type TaskAnalysis struct {
	GoalID              string   `json:"goal_id"`
	Complexity          string   `json:"complexity"`           // low | medium | high | extreme
	RequiredCapabilities []string `json:"required_capabilities"` // 需要的 capability action_types
	SuggestedFlow       string   `json:"suggested_flow"`       // 推荐的 Flow 模板名（如 "code-project-v1"）
	RiskAssessment      string   `json:"risk_assessment"`      // L0-L5 风险等级
	EstimatedSteps      int      `json:"estimated_steps"`      // 预估步骤数
	Reasoning           string   `json:"reasoning"`            // 推荐理由
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
	flowName, _ := evt.Payload["flow_name"].(string) // v0.1.0: Flow 模板约束

	ctx := Context{
		GoalID:      evt.GoalID,
		GoalText:    goalText,
		AnchorCheck: anchorCheck,
	}

	// v0.1.0 三步规划（R-350）：Align → Analyze → Plan
	criteria, err := e.agent.Align(goalText, ctx)
	if err != nil {
		log.Printf("[MissionEngine] Agent.Align failed: %v", err)
		if isTimeout(err) {
			// R-387: LLM 超时 → 诚实失败。不降级 PlanLegacy，不伪造产出物
			e.publishGoalFailed(evt.GoalID, "LLM 规划超时（Align 阶段）。建议：换用更快的模型，或简化目标描述。")
			return nil
		}
		// LLM 完全不可用 → PlanLegacy 最小可用路径
		graph, err := e.agent.PlanLegacy(goalText, ctx)
		if err != nil {
			e.publishRejected(evt.GoalID, err.Error(), 1)
			return nil
		}
		e.publishGraph(evt.GoalID, graph)
		return nil
	}

	analysis, err := e.agent.Analyze(criteria, ctx)
	if err != nil {
		log.Printf("[MissionEngine] Agent.Analyze failed: %v", err)
		if isTimeout(err) {
			e.publishGoalFailed(evt.GoalID, "LLM 规划超时（Analyze 阶段）。建议：换用更快的模型，或简化目标描述。")
			return nil
		}
		graph, err := e.agent.Plan(criteria, nil, flowName, ctx)
		if err != nil {
			e.publishRejected(evt.GoalID, err.Error(), 1)
			return nil
		}
		e.publishGraph(evt.GoalID, graph)
		return nil
	}

	// 如果 FlowRecommender 未指定模板，使用 Agent 推荐的 Flow
	if flowName == "" {
		flowName = analysis.SuggestedFlow
	}

	graph, err := e.agent.Plan(criteria, analysis, flowName, ctx)
	if err != nil {
		log.Printf("[MissionEngine] Agent.Plan failed: %v", err)
		if isTimeout(err) {
			// LLM 超时→发布干预事件+GoalFailed（诚实反馈：不伪造产出物）
			e.publishTimeoutIntervention(evt.GoalID, goalText, "Plan", err)
			e.publish(events.Event{
				Type: events.TypeGoalFailed, GoalID: evt.GoalID, Source: "mission-engine",
				Payload: map[string]interface{}{"reason": "llm_timeout", "error": "LLM 规划超时，请重试或更换模型"},
			})
			return nil
		}
		e.publishRejected(evt.GoalID, err.Error(), 1)
		return nil
	}

	// Validate and publish
	if err := e.validate(graph); err != nil {
		log.Printf("[MissionEngine] validation failed: %v", err)
		e.publishRejected(evt.GoalID, err.Error(), 1)
		return nil
	}

	e.publishGraph(evt.GoalID, graph)
	return nil
}

// publishRejected 发布 MissionGraphRejected 事件。
func (e *Engine) publishRejected(goalID string, reason string, attempt int) {
	e.publish(events.Event{
		Type:   events.TypeMissionGraphRejected,
		GoalID: goalID,
		Source: "mission-engine",
		Payload: map[string]interface{}{
			"error":   reason,
			"attempt": float64(attempt),
		},
	})
}

// publishGraph 发布 MissionGenerated + UserConfirmed 事件。
func (e *Engine) publishGraph(goalID string, graph *MissionGraph) {
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
		GoalID: goalID,
		Source: "mission-engine",
		Payload: map[string]interface{}{
			"node_count": float64(len(graph.Nodes)),
			"strategy":   "GoalAgent",
			"nodes":      nodesPayload,
		},
	})

	// 驱动状态机：自动确认（MVP 无人工确认环节）
	e.publish(events.Event{
		Type:   events.TypeUserConfirmed,
		GoalID: goalID,
		Source: "mission-engine",
	})
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

// publishGoalFailed 发布 GoalFailed 事件（R-387 诚实失败：含可操作建议）。
func (e *Engine) publishGoalFailed(goalID string, reason string) {
	e.publish(events.Event{
		Type:   events.TypeGoalFailed,
		GoalID: goalID,
		Source: "mission-engine",
		Payload: map[string]interface{}{
			"reason": reason,
			"error":  "llm_timeout",
		},
	})
}

// isTimeout 检测 LLM 调用是否因超时取消（v0.1.1 Jobs 产品决策：超时→用户选择，非自动降级）。
func isTimeout(err error) bool {
	if err == nil {
		return false
	}
	s := err.Error()
	return strings.Contains(s, "deadline exceeded") ||
		strings.Contains(s, "timeout") ||
		strings.Contains(s, "canceled")
}

// publishTimeoutIntervention 发布 LLM 超时干预事件，让用户选择下一步（v0.1.1 Jobs 产品决策）。
func (e *Engine) publishTimeoutIntervention(goalID string, goalText string, stage string, err error) {
	log.Printf("[MissionEngine] LLM timeout at %s stage — asking user to decide", stage)
	e.publish(events.Event{
		Type:   events.TypeHumanInterventionRequested,
		GoalID: goalID,
		Source: "mission-engine",
		Payload: map[string]interface{}{
			"reason":       fmt.Sprintf("LLM 超时 (%s阶段): %v", stage, err),
			"stage":        stage,
			"goal_text":    goalText,
			"intervention_type": "llm_timeout",
			"options": []map[string]string{
				{"action": "keep_waiting", "label": "继续等待", "desc": "保持当前模型，延长超时时间继续"},
				{"action": "simplify", "label": "简化方案", "desc": "使用系统默认方案快速完成"},
				{"action": "switch_model", "label": "更换模型", "desc": "换一个更快的模型重试"},
				{"action": "cancel", "label": "取消目标", "desc": "不再执行此目标"},
			},
		},
	})
}

// StubAgent 硬编码单节点图，用于无 LLM 环境下的核心链路测试。
// 配置 LLM Provider 后自动切换到 GoalAgent。
type StubAgent struct{}

// NewStubAgent 创建 StubAgent（默认 Agent，零外部依赖）。
func NewStubAgent() *StubAgent { return &StubAgent{} }

// Align 返回默认完成标准（Stub 实现）。
func (s *StubAgent) Align(goal string, ctx Context) (*CompletionCriteria, error) {
	return &CompletionCriteria{
		GoalID:            ctx.GoalID,
		GoalType:          "other",
		SuccessDefinition: goal,
		Complexity:        "medium",
	}, nil
}

// Analyze 返回默认任务分析（Stub 实现）。
func (s *StubAgent) Analyze(criteria *CompletionCriteria, ctx Context) (*TaskAnalysis, error) {
	return &TaskAnalysis{
		GoalID:        ctx.GoalID,
		Complexity:    "medium",
		SuggestedFlow: "generic-v1",
		RiskAssessment: "L1",
		EstimatedSteps: 1,
	}, nil
}

// Plan 生成单节点 MissionGraph（Stub 实现）。
func (s *StubAgent) Plan(criteria *CompletionCriteria, analysis *TaskAnalysis, flowName string, ctx Context) (*MissionGraph, error) {
	goal := ctx.GoalText
	if criteria != nil && criteria.SuccessDefinition != "" {
		goal = criteria.SuccessDefinition
	}
	actionType, target := InferAction(goal)
	return &MissionGraph{
		GoalID: ctx.GoalID,
		Nodes:  []GraphNode{{ID: "1", Type: "mission", Description: goal, ActionType: actionType, Target: target}},
		Edges:  []GraphEdge{},
	}, nil
}

// PlanLegacy 旧版接口（W3 废弃）。
func (s *StubAgent) PlanLegacy(goal string, ctx Context) (*MissionGraph, error) {
	return s.Plan(nil, nil, "", ctx)
}

// Verify Stub 实现（v0.1.0 R-372）。
func (s *StubAgent) Verify(code string, actionID string, ctx Context) (*VerificationResult, error) {
	if len(code) == 0 {
		return &VerificationResult{ActionID: actionID, Verdict: "FAIL", Reason: "empty code", Score: 0}, nil
	}
	return &VerificationResult{ActionID: actionID, Verdict: "PASS", Reason: "stub", Score: 100}, nil
}

// InferAction 返回默认 action_type。v0.1.0: GoalAgent+LLM 推理替代关键词匹配。
// 仅作为 StubAgent/fallbackPlan 的最后回退。
func InferAction(goal string) (string, string) {
	return "shell.execute", goal
}

// SetAgent 热替换 Agent（v0.1.0 UX1 热加载）。
// 线程安全。可在运行时切换 LLM Provider/Model 而不重启 daemon。
func (e *Engine) SetAgent(agent Agent) {
	e.agent = agent
	log.Printf("[MissionEngine] agent hot-swapped to %T", agent)
}

