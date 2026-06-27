// MissionEngine 校验测试 — 拓扑排序、循环检测、边引用。
package missionengine_test

import (
	"testing"
	"time"

	"github.com/goalos/goalos/internal/eventbus"
	"github.com/goalos/goalos/internal/missionengine"
	"github.com/goalos/goalos/pkg/events"
)

// TestMissionGraph_CycleRejected 验证循环依赖被 MissionGraphRejected。
func TestMissionGraph_CycleRejected(t *testing.T) {
	bus := eventbus.New()
	eng := missionengine.New(bus, &cycleAgent{})
	eng.Start()

	rejected := make(chan events.Event, 1)
	bus.Subscribe(events.TypeMissionGraphRejected, func(evt events.Event) error {
		rejected <- evt
		return nil
	})

	bus.Publish(events.Event{
		Type:   events.TypePlanRequested,
		GoalID: "goal_cycle",
		Source: "scheduler",
		Payload: map[string]interface{}{
			"goal_text":         "test cycle",
			"goal_anchor_check": false,
		},
	})

	select {
	case evt := <-rejected:
		// reject_reasons is []string in payload (not []interface{})
		switch v := evt.Payload["reject_reasons"].(type) {
		case []string:
			if len(v) == 0 {
				t.Error("reject_reasons should not be empty")
			}
			t.Logf("cycle rejected: %v", v)
		case []interface{}:
			if len(v) == 0 {
				t.Error("reject_reasons should not be empty")
			}
			t.Logf("cycle rejected: %v", v)
		default:
			t.Logf("cycle rejected (reasons type: %T)", evt.Payload["reject_reasons"])
		}
	case <-time.After(time.Second):
		t.Fatal("带循环的 MissionGraph 应触发 MissionGraphRejected")
	}
}

// cycleAgent returns a graph with cycle: 1→2→3→1.
type cycleAgent struct{}

func (a *cycleAgent) Plan(criteria *missionengine.CompletionCriteria, analysis *missionengine.TaskAnalysis, flowName string, ctx missionengine.Context) (*missionengine.MissionGraph, error) {
	return &missionengine.MissionGraph{
		GoalID: ctx.GoalID,
		Nodes: []missionengine.GraphNode{
			{ID: "1", Type: "mission", Description: "step 1"},
			{ID: "2", Type: "mission", Description: "step 2"},
			{ID: "3", Type: "mission", Description: "step 3"},
		},
		Edges: []missionengine.GraphEdge{
			{From: "1", To: "2", Type: "sequential"},
			{From: "2", To: "3", Type: "sequential"},
			{From: "3", To: "1", Type: "sequential"}, // back-edge → cycle
		},
	}, nil
}

// TestMissionGraph_InvalidEdgeDropped 验证边引用不存在的边被静默丢弃（LLM 输出容错）。
// validate 函数会过滤掉引用不存在节点的边，不拒绝整个 Graph。
func TestMissionGraph_InvalidEdgeDropped(t *testing.T) {
	bus := eventbus.New()
	eng := missionengine.New(bus, &badEdgeAgent{})
	eng.Start()

	generated := make(chan events.Event, 1)
	bus.Subscribe(events.TypeMissionGenerated, func(evt events.Event) error {
		generated <- evt
		return nil
	})

	bus.Publish(events.Event{
		Type:   events.TypePlanRequested,
		GoalID: "goal_badedge",
		Source: "scheduler",
		Payload: map[string]interface{}{
			"goal_text":         "test bad edge",
			"goal_anchor_check": false,
		},
	})

	select {
	case evt := <-generated:
		// ok — validate 过滤了无效边，graph 仍然生成
		nodes, _ := evt.Payload["nodes"].([]interface{})
		if len(nodes) != 1 {
			t.Errorf("expected 1 valid node, got %d", len(nodes))
		}
	case <-time.After(time.Second):
		t.Fatal("应该生成 MissionGraph（无效边被过滤），但超时了")
	}
}

type badEdgeAgent struct{}

func (a *badEdgeAgent) Plan(criteria *missionengine.CompletionCriteria, analysis *missionengine.TaskAnalysis, flowName string, ctx missionengine.Context) (*missionengine.MissionGraph, error) {
	return &missionengine.MissionGraph{
		GoalID: ctx.GoalID,
		Nodes: []missionengine.GraphNode{
			{ID: "1", Type: "mission", Description: "step 1"},
		},
		Edges: []missionengine.GraphEdge{
			{From: "1", To: "nonexistent", Type: "sequential"},
		},
	}, nil
}

// TestMissionGraph_SelfLoopDropped 验证自循环边被静默丢弃（LLM 输出容错）。
// validate 函数会过滤掉自循环边（from == to），不拒绝整个 Graph。
func TestMissionGraph_SelfLoopDropped(t *testing.T) {
	bus := eventbus.New()
	eng := missionengine.New(bus, &selfLoopAgent{})
	eng.Start()

	generated := make(chan events.Event, 1)
	bus.Subscribe(events.TypeMissionGenerated, func(evt events.Event) error {
		generated <- evt
		return nil
	})

	bus.Publish(events.Event{
		Type:   events.TypePlanRequested,
		GoalID: "goal_selfloop",
		Source: "scheduler",
		Payload: map[string]interface{}{
			"goal_text":         "test self-loop",
			"goal_anchor_check": false,
		},
	})

	select {
	case <-generated:
		// ok — validate 过滤了自循环边，graph 仍然生成
	case <-time.After(time.Second):
		t.Fatal("应该生成 MissionGraph（自循环边被过滤），但超时了")
	}
}

type selfLoopAgent struct{}

func (a *selfLoopAgent) Plan(criteria *missionengine.CompletionCriteria, analysis *missionengine.TaskAnalysis, flowName string, ctx missionengine.Context) (*missionengine.MissionGraph, error) {
	return &missionengine.MissionGraph{
		GoalID: ctx.GoalID,
		Nodes: []missionengine.GraphNode{
			{ID: "1", Type: "mission", Description: "step 1"},
			{ID: "2", Type: "mission", Description: "step 2"},
		},
		Edges: []missionengine.GraphEdge{
			{From: "1", To: "1", Type: "sequential"},
		},
	}, nil
}

func (a *cycleAgent) Align(goal string, ctx missionengine.Context) (*missionengine.CompletionCriteria, error) {
	return &missionengine.CompletionCriteria{GoalID: ctx.GoalID, GoalType: "other", SuccessDefinition: goal, Complexity: "medium"}, nil
}
func (a *cycleAgent) Analyze(criteria *missionengine.CompletionCriteria, ctx missionengine.Context) (*missionengine.TaskAnalysis, error) {
	return &missionengine.TaskAnalysis{GoalID: ctx.GoalID, Complexity: "medium", SuggestedFlow: "generic-v1"}, nil
}

func (a *badEdgeAgent) Align(goal string, ctx missionengine.Context) (*missionengine.CompletionCriteria, error) {
	return &missionengine.CompletionCriteria{GoalID: ctx.GoalID, GoalType: "other", SuccessDefinition: goal, Complexity: "medium"}, nil
}
func (a *badEdgeAgent) Analyze(criteria *missionengine.CompletionCriteria, ctx missionengine.Context) (*missionengine.TaskAnalysis, error) {
	return &missionengine.TaskAnalysis{GoalID: ctx.GoalID, Complexity: "medium", SuggestedFlow: "generic-v1"}, nil
}

func (a *selfLoopAgent) Align(goal string, ctx missionengine.Context) (*missionengine.CompletionCriteria, error) {
	return &missionengine.CompletionCriteria{GoalID: ctx.GoalID, GoalType: "other", SuccessDefinition: goal, Complexity: "medium"}, nil
}
func (a *selfLoopAgent) Analyze(criteria *missionengine.CompletionCriteria, ctx missionengine.Context) (*missionengine.TaskAnalysis, error) {
	return &missionengine.TaskAnalysis{GoalID: ctx.GoalID, Complexity: "medium", SuggestedFlow: "generic-v1"}, nil
}

func (a *cycleAgent) PlanLegacy(goal string, ctx missionengine.Context) (*missionengine.MissionGraph, error) {
	return a.Plan(nil, nil, "", ctx)
}
func (a *badEdgeAgent) PlanLegacy(goal string, ctx missionengine.Context) (*missionengine.MissionGraph, error) {
	return a.Plan(nil, nil, "", ctx)
}
func (a *selfLoopAgent) PlanLegacy(goal string, ctx missionengine.Context) (*missionengine.MissionGraph, error) {
	return a.Plan(nil, nil, "", ctx)
}
