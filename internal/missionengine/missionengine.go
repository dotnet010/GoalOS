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

// Agent is the planning interface. MVP: StubAgent. W3: GoalAgent.
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
	Type        string `json:"type"` // "mission" | "action" | "approval" | "condition" | "sub_goal" | "clarification"
	Description string `json:"description"`
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

	e.publish(events.Event{
		Type:   events.TypeMissionGenerated,
		GoalID: evt.GoalID,
		Source: "mission-engine",
		Payload: map[string]interface{}{
			"node_count": float64(len(graph.Nodes)),
			"strategy":   "GoalAgent",
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
	// W1: basic validation. Full validation per R227.
	return nil
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

// StubAgent is the W1 hardcoded Agent. Returns a 3-node MissionGraph.
// Replaced by GoalAgent (LLM-powered) in W3.
type StubAgent struct{}

func (s *StubAgent) Plan(goal string, ctx Context) (*MissionGraph, error) {
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
	}, nil
}
