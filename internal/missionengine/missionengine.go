// Package missionengine implements the GoalOS Mission Engine.
// 订阅 PlanRequested → 调用 Agent.plan() → 校验 MissionGraph → 发布 MissionGenerated/MissionGraphRejected。
//
// 设计依据：05 架构文档 §5、R153、R227。
package missionengine

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net"
	"sort"

	"github.com/goalos/goalos/internal/eventbus"
	"github.com/goalos/goalos/internal/trace"
	"github.com/goalos/goalos/pkg/events"
)

// Agent is the planning + verification interface (v0.1.0: Planner + Verifier 双角色）。
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
	// R-724: PlanLegacy 已从接口移除——LLM 失败即诚实失败

	// ── Verifier 角色（v0.1.0 R-372）──
	// Verify 对产出代码进行验证。由 PipelineRunner Check 原语通过 QualityGate 调用。
	Verify(code string, actionID string, ctx Context) (*VerificationResult, error)
}

// VerificationResult 是 Agent.Verify() 的返回结果（v0.1.0 R-372）。
type VerificationResult struct {
	ActionID string `json:"action_id"`
	Verdict  string `json:"verdict"` // "PASS" | "WARN" | "FAIL"
	Reason   string `json:"reason"`  // 判定理由
	Score    int    `json:"score"`   // 0-100
}

// Context is the planning context.
// R-741（新 Session 重做——会议 #107）：Agent 在开启新 Session 重新 Plan 时能够感知已完成的
// 产出物，避免重复规划与重复执行——CompletedArtifacts 字典+ExecutionHistory 结构入 Context。
type Context struct {
	GoalID      string
	GoalText    string
	AnchorCheck bool

	// CompletedArtifacts — 已完成 Action 的产出物字典（R-741 新 Session 重做）。
	// 契约：key=ActionID，value=产出物路径列表（artifact_paths）——Agent 重新 Plan 时
	// 感知已完成产出物，避免重复规划与重复执行。
	CompletedArtifacts map[string][]string

	// ExecutionHistory — 执行历史结构（R-741 新 Session 重做）。
	// 契约：已完成 Action 的执行记录（ActionID/Status/ArtifactPaths/Timestamp）——
	// Agent 重新 Plan 时感知执行历史，基于当前上下文+失败原因重新规划。
	ExecutionHistory []ExecutionRecord
}

// ExecutionRecord — 执行记录（已完成 Action 的历史）。
type ExecutionRecord struct {
	ActionID      string   // Action ID
	Status        string   // 执行状态（Completed/Failed/...）
	ArtifactPaths []string // 产出物路径列表（artifact_paths）
	Timestamp     string   // 完成时间戳
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
	GoalID               string   `json:"goal_id"`
	Complexity           string   `json:"complexity"`            // low | medium | high | extreme
	RequiredCapabilities []string `json:"required_capabilities"` // 需要的 capability action_types
	SuggestedFlow        string   `json:"suggested_flow"`        // 推荐的 Flow 模板名（如 "code-project-v1"）
	RiskAssessment       string   `json:"risk_assessment"`       // L0-L5 风险等级
	EstimatedSteps       int      `json:"estimated_steps"`       // 预估步骤数
	Reasoning            string   `json:"reasoning"`             // 推荐理由
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
	bus           *eventbus.EventBus
	agent         Agent
	fallbackAgent Agent // v0.2.2 W8 B13: Plan 失败时的回退 Provider
	seq           int
	flowComposer  interface{} // A18: FlowComposer 验证（*scheduler.FlowComposer）
	autoConfirm   bool        // v0.2.2 W6 B10: autonomous 模式自动确认
}

// SetFallbackAgent 设置 Plan 阶段的回退 Provider（B13）。
func (e *Engine) SetFallbackAgent(a Agent) { e.fallbackAgent = a }

// New creates a Mission Engine.
func New(bus *eventbus.EventBus, agent Agent) *Engine {
	return &Engine{bus: bus, agent: agent}
}

// SetFlowComposer 设置 FlowComposer（A18）。
func (e *Engine) SetFlowComposer(fc interface{}) {
	e.flowComposer = fc
}

// SetAutoConfirm 设置是否自动确认 MissionGraph（B10）。
// autonomous 模式 → true。否则等待用户确认。
func (e *Engine) SetAutoConfirm(auto bool) {
	e.autoConfirm = auto
}

// Start subscribes to PlanRequested and begins processing.
func (e *Engine) Start() {
	e.bus.Subscribe(events.TypePlanRequested, e.handlePlanRequested)
	log.Println("[MissionEngine] started, subscribed to PlanRequested")
}

// handlePlanRequested 处理 PlanRequested 事件。
// [FIXED] 原代码：Analyze 失败时仍调用 Plan 并发布图（假成功）
// [FIXED] 现在：任何阶段失败都终止流程并发布 GoalFailed
func (e *Engine) handlePlanRequested(evt events.Event) error {
	t := trace.Start(evt.GoalID)
	t.StageStart("PlanRequested.received")
	log.Printf("[MissionEngine] handlePlanRequested RECEIVED: goal=%s text=%.50s", evt.GoalID, fmt.Sprint(evt.Payload["goal_text"]))
	goalText, _ := evt.Payload["goal_text"].(string)
	anchorCheck, _ := evt.Payload["goal_anchor_check"].(bool)
	flowName, _ := evt.Payload["flow_name"].(string) // v0.1.0: Flow 模板约束

	ctx := Context{
		GoalID:      evt.GoalID,
		GoalText:    goalText,
		AnchorCheck: anchorCheck,
	}

	// v0.1.0 三步规划（R-350）：Align → Analyze → Plan
	// R-724: 删除 PlanLegacy 回退——LLM 失败即诚实失败，不伪造产出物
	// R-835: 推送进度——LLM Align 开始
	e.pushProgress(evt.GoalID, "Align", "LLM 正在分析目标...")
	t.StageStart("Agent.Align")
	criteria, err := e.agent.Align(goalText, ctx)
	if err != nil {
		t.StageFail("Agent.Align", err)
		t.Summary()
		e.publishGoalFailed(evt.GoalID, fmt.Sprintf("LLM 规划失败（Align 阶段）: %v", err))
		return nil
	}
	// [FIXED] 防御性校验：即使 err == nil，也要验证返回值不为 nil
	if criteria == nil {
		err := fmt.Errorf("Agent.Align 返回了 nil criteria 但没有错误")
		t.StageFail("Agent.Align", err)
		t.Summary()
		e.publishGoalFailed(evt.GoalID, "LLM 规划失败（Align 阶段返回空数据）")
		return nil
	}
	t.StageOK("Agent.Align")

	// [FIXED] 原代码：Analyze 失败时（err != nil），仍调用 Plan(criteria, nil, ...) 并发布图
	// 这是致命 bug：Analyze 失败意味着没有有效的分析结果，但系统仍生成"假任务图"
	// [FIXED] 现在：Analyze 失败即终止流程，发布 GoalFailed
	e.pushProgress(evt.GoalID, "Analyze", "LLM 正在评估任务复杂度...")
	t.StageStart("Agent.Analyze")
	analysis, err := e.agent.Analyze(criteria, ctx)
	if err != nil {
		t.StageFail("Agent.Analyze", err)
		t.Summary()
		if isTimeout(err) {
			e.publishGoalFailed(evt.GoalID, "LLM 规划超时（Analyze 阶段）。建议：换用更快的模型，或简化目标描述。")
		} else {
			e.publishGoalFailed(evt.GoalID, fmt.Sprintf("LLM 规划失败（Analyze 阶段）: %v", err))
		}
		return nil
	}
	// [FIXED] 防御性校验
	if analysis == nil {
		err := fmt.Errorf("Agent.Analyze 返回了 nil analysis 但没有错误")
		t.StageFail("Agent.Analyze", err)
		t.Summary()
		e.publishGoalFailed(evt.GoalID, "LLM 规划失败（Analyze 阶段返回空数据）")
		return nil
	}
	t.StageOK("Agent.Analyze")

	// 如果 FlowRecommender 未指定模板，使用 Agent 推荐的 Flow
	if flowName == "" {
		flowName = analysis.SuggestedFlow
	}

	e.pushProgress(evt.GoalID, "Plan", "LLM 正在生成执行计划...")
	t.StageStart("Agent.Plan")
	graph, err := e.agent.Plan(criteria, analysis, flowName, ctx)
	// B13: Plan 失败→尝试回退 Provider
	if err != nil && e.fallbackAgent != nil {
		log.Printf("[MissionEngine] Plan failed with primary agent: %v — trying fallback", err)
		graph, err = e.fallbackAgent.Plan(criteria, analysis, flowName, ctx)
	}
	if err != nil {
		t.StageFail("Agent.Plan", err)
		t.Summary()
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
	// [FIXED] 防御性校验
	if graph == nil {
		err := fmt.Errorf("Agent.Plan 返回了 nil graph 但没有错误")
		t.StageFail("Agent.Plan", err)
		t.Summary()
		e.publishGoalFailed(evt.GoalID, "LLM 规划失败（Plan 阶段返回空数据）")
		return nil
	}
	t.StageOK("Agent.Plan")

	// Validate and publish
	t.StageStart("MissionGraph.validate")
	if err := e.validate(graph); err != nil {
		t.StageFail("MissionGraph.validate", err)
		t.Summary()
		log.Printf("[MissionEngine] validation failed: %v", err)
		e.publishRejected(evt.GoalID, err.Error(), 1)
		return nil
	}
	t.StageOK("MissionGraph.validate")

	t.Summary()
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

// PlanHash 计算 MissionGraph 的规范 JSON SHA256 哈希（R-859）。
// 使用 sorted keys + compact JSON 确保确定性。
func PlanHash(graph *MissionGraph) string {
	// 构建可排序的规范化结构
	type nodeRep struct {
		ID          string `json:"id"`
		Type        string `json:"type"`
		Description string `json:"description"`
		ActionType  string `json:"action_type"`
		Target      string `json:"target"`
	}
	nodes := make([]nodeRep, len(graph.Nodes))
	for i, n := range graph.Nodes {
		nodes[i] = nodeRep{n.ID, n.Type, n.Description, n.ActionType, n.Target}
	}
	// 按 ID 排序确保确定性
	sort.Slice(nodes, func(i, j int) bool { return nodes[i].ID < nodes[j].ID })

	data, _ := json.Marshal(map[string]interface{}{
		"node_count": len(graph.Nodes),
		"nodes":      nodes,
	})
	h := sha256.Sum256(data)
	return fmt.Sprintf("%x", h)
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

	// R-859: 计算 PlanHash，写入 MissionGenerated 事件
	ph := PlanHash(graph)
	log.Printf("[MissionEngine] PlanHash: %s (goal=%s, nodes=%d)", ph[:12], goalID, len(graph.Nodes))

	e.publish(events.Event{
		Type:   events.TypeMissionGenerated,
		GoalID: goalID,
		Source: "mission-engine",
		Payload: map[string]interface{}{
			"node_count": float64(len(graph.Nodes)),
			"strategy":   "GoalAgent",
			"nodes":      nodesPayload,
			"plan_hash":  ph, // R-859
		},
	})

	// v0.2.2 W6 B10: autonomous 模式自动确认，否则等待用户确认
	if e.autoConfirm {
		e.publish(events.Event{
			Type:   events.TypeUserConfirmed,
			GoalID: goalID,
			Source: "mission-engine",
		})
	} else {
		// W6-K1: 非 autonomous 模式不自动确认——发布 UserRejected 避免死锁。
		// Goal 不会挂起；Scheduler 收到 UserRejected 后通知用户需手动确认。
		log.Printf("[MissionEngine] non-autonomous mode: publishing UserRejected for goal=%s", goalID)
		e.publish(events.Event{
			Type:   events.TypeUserRejected,
			GoalID: goalID,
			Source: "mission-engine",
			Payload: map[string]interface{}{
				"reason": "manual_confirmation_required",
			},
		})
	}
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
			"error":  "llm_failure",
		},
	})
}

// isTimeout 检测 LLM 调用是否因超时取消（v0.1.1 Jobs 产品决策：超时→用户选择，非自动降级）。
func isTimeout(err error) bool {
	if err == nil {
		return false
	}
	// E5: 使用 errors.Is 而非字符串匹配
	if errors.Is(err, context.DeadlineExceeded) || errors.Is(err, context.Canceled) {
		return true
	}
	// 兜底：net.Error 超时
	var netErr net.Error
	if errors.As(err, &netErr) && netErr.Timeout() {
		return true
	}
	return false
}

// pushProgress 推送 Plan 阶段进度到 EventBus（R-835: 实时反馈——SSE /api/events 事件流，CLI 消费；Dashboard 已拆除 R-1372）。
func (e *Engine) pushProgress(goalID, stage, detail string) {
	e.publish(events.Event{
		Type:   "PlanProgressUpdate",
		GoalID: goalID,
		Source: "mission-engine",
		Payload: map[string]interface{}{
			"stage":  stage,
			"detail": detail,
		},
	})
}

// publishTimeoutIntervention 发布 LLM 超时干预事件，让用户选择下一步（v0.1.1 Jobs 产品决策）。
func (e *Engine) publishTimeoutIntervention(goalID string, goalText string, stage string, err error) {
	log.Printf("[MissionEngine] LLM timeout at %s stage — asking user to decide", stage)
	e.publish(events.Event{
		Type:   events.TypeHumanInterventionRequested,
		GoalID: goalID,
		Source: "mission-engine",
		Payload: map[string]interface{}{
			"reason":            fmt.Sprintf("LLM 超时 (%s阶段): %v", stage, err),
			"stage":             stage,
			"goal_text":         goalText,
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
// [WARNING] StubAgent 返回硬编码数据，仅用于测试/CI，生产环境必须使用 GoalAgent。
type StubAgent struct{}

// NewStubAgent 创建 StubAgent（默认 Agent，零外部依赖）。
func NewStubAgent() *StubAgent { return &StubAgent{} }

// Align 返回默认完成标准（Stub 实现）。
// [WARNING] 这是测试桩，生产环境不应使用
func (s *StubAgent) Align(goal string, ctx Context) (*CompletionCriteria, error) {
	return &CompletionCriteria{
		GoalID:            ctx.GoalID,
		GoalType:          "other",
		SuccessDefinition: goal,
		Complexity:        "medium",
	}, nil
}

// Analyze 返回默认任务分析（Stub 实现）。
// [WARNING] 这是测试桩，生产环境不应使用
func (s *StubAgent) Analyze(criteria *CompletionCriteria, ctx Context) (*TaskAnalysis, error) {
	return &TaskAnalysis{
		GoalID:         ctx.GoalID,
		Complexity:     "medium",
		SuggestedFlow:  "generic-v1",
		RiskAssessment: "L1",
		EstimatedSteps: 1,
	}, nil
}

// Plan 生成单节点 MissionGraph（Stub 实现）。
// [WARNING] 这是测试桩，生产环境不应使用
func (s *StubAgent) Plan(criteria *CompletionCriteria, analysis *TaskAnalysis, flowName string, ctx Context) (*MissionGraph, error) {
	goal := ctx.GoalText
	if criteria != nil && criteria.SuccessDefinition != "" {
		goal = criteria.SuccessDefinition
	}
	actionType, target, err := InferAction(goal)
	if err != nil {
		log.Printf("[WARNING] STUB AGENT: cannot infer action type for goal: %v. Returning minimal graph.", err)
		return &MissionGraph{
			GoalID: ctx.GoalID,
			Nodes:  []GraphNode{{ID: "1", Type: "mission", Description: goal, ActionType: "unknown", Target: goal}},
			Edges:  []GraphEdge{},
		}, nil
	}
	return &MissionGraph{
		GoalID: ctx.GoalID,
		Nodes:  []GraphNode{{ID: "1", Type: "mission", Description: goal, ActionType: actionType, Target: target}},
		Edges:  []GraphEdge{},
	}, nil
}

// R-724: PlanLegacy 已删除——LLM 失败即诚实失败。

// Verify Stub 实现（v0.1.0 R-372）。
// [WARNING] 这是测试桩，生产环境不应使用
// v0.2.0 audit fix: 非空代码返回 WARN 而非 PASS，明确标注无法验证。
func (s *StubAgent) Verify(code string, actionID string, ctx Context) (*VerificationResult, error) {
	if len(code) == 0 {
		return &VerificationResult{ActionID: actionID, Verdict: "FAIL", Reason: "stub: empty code — cannot verify", Score: 0}, nil
	}
	return &VerificationResult{
		ActionID: actionID,
		Verdict:  "WARN",
		Reason:   "stub agent active — no real verification performed. Result confidence is 0.",
		Score:    0,
	}, nil
}

// InferAction 返回默认 action_type。v0.1.0: GoalAgent+LLM 推理替代关键词匹配。
// 仅作为 StubAgent 的最后回退。调用方必须检查 error。
// v0.2.0 audit fix: 返回 error 而非静默 fallback 到 shell.execute。
// [WARNING] 生产环境应使用 GoalAgent 的 LLM 推理，而非此硬编码回退
func InferAction(goal string) (string, string, error) {
	return "", "", fmt.Errorf("cannot infer action: no LLM agent configured for goal: %s", goal)
}

// SetAgent 热替换 Agent（v0.1.0 UX1 热加载）。
// 线程安全。可在运行时切换 LLM Provider/Model 而不重启 daemon。
func (e *Engine) SetAgent(agent Agent) {
	e.agent = agent
	log.Printf("[MissionEngine] agent hot-swapped to %T", agent)
}
