package missionengine_test

import (
	"testing"
	"time"

	"github.com/goalos/goalos/internal/eventbus"
	"github.com/goalos/goalos/internal/missionengine"
	"github.com/goalos/goalos/pkg/events"
)

func TestMissionEngine_PlanRequested(t *testing.T) {
	bus := eventbus.New()
	engine := missionengine.New(bus, &missionengine.StubAgent{})
	engine.Start()

	generated := make(chan events.Event, 1)
	bus.Subscribe(events.TypeMissionGenerated, func(evt events.Event) error {
		generated <- evt
		return nil
	})

	bus.Publish(events.Event{
		Type:   events.TypePlanRequested,
		GoalID: "goal_001",
		Source: "scheduler",
		Payload: map[string]interface{}{
			"goal_text":         "开发CRM系统",
			"goal_anchor_check": false,
		},
	})

	select {
	case evt := <-generated:
		nodeCount, _ := evt.Payload["node_count"].(float64)
		if int(nodeCount) < 1 {
			t.Errorf("expected >=1 nodes, got %d", int(nodeCount))
		}
		// 验证 nodes payload 包含 action_type 字段
		if nodes, ok := evt.Payload["nodes"].([]interface{}); ok && len(nodes) > 0 {
			node := nodes[0].(map[string]interface{})
			t.Logf("node action_type=%s target=%s", node["action_type"], node["target"])
		}
	default:
		t.Fatal("MissionGenerated event was not published")
	}
}

func TestMissionEngine_EmptyGraphRejected(t *testing.T) {
	bus := eventbus.New()
	emptyAgent := &emptyStubAgent{}
	engine := missionengine.New(bus, emptyAgent)
	engine.Start()

	rejected := make(chan events.Event, 1)
	bus.Subscribe(events.TypeMissionGraphRejected, func(evt events.Event) error {
		rejected <- evt
		return nil
	})

	bus.Publish(events.Event{
		Type:   events.TypePlanRequested,
		GoalID: "goal_002",
		Source: "scheduler",
		Payload: map[string]interface{}{
			"goal_text": "test",
		},
	})

	select {
	case <-rejected:
	default:
		t.Fatal("MissionGraphRejected was not published for empty graph")
	}
}

// TestMissionEngine_WebSearchActionType 验证搜索类 Goal 产生 shell.execute action_type。
func TestMissionEngine_WebSearchActionType(t *testing.T) {
	bus := eventbus.New()
	engine := missionengine.New(bus, &missionengine.StubAgent{})
	engine.Start()

	generated := make(chan events.Event, 1)
	bus.Subscribe(events.TypeMissionGenerated, func(evt events.Event) error {
		generated <- evt
		return nil
	})

	bus.Publish(events.Event{
		Type:   events.TypePlanRequested,
		GoalID: "goal_search",
		Source: "scheduler",
		Payload: map[string]interface{}{
			"goal_text":         "搜索AI新闻",
			"goal_anchor_check": false,
		},
	})

	select {
	case evt := <-generated:
		nodes, _ := evt.Payload["nodes"].([]interface{})
		if len(nodes) < 1 {
			t.Fatal("expected at least 1 node")
		}
		node := nodes[0].(map[string]interface{})
		actionType, _ := node["action_type"].(string)
		if actionType != "shell.execute" {
			t.Errorf("搜索类 Goal 应映射到 shell.execute, got %s", actionType)
		}
	case <-time.After(time.Second):
		t.Fatal("MissionGenerated event was not published for search goal (timeout)")
	}
}

type emptyStubAgent struct{}

func (s *emptyStubAgent) Align(goal string, ctx missionengine.Context) (*missionengine.CompletionCriteria, error) {
	return &missionengine.CompletionCriteria{GoalID: ctx.GoalID, GoalType: "other", SuccessDefinition: goal, Complexity: "medium"}, nil
}
func (s *emptyStubAgent) Analyze(criteria *missionengine.CompletionCriteria, ctx missionengine.Context) (*missionengine.TaskAnalysis, error) {
	return &missionengine.TaskAnalysis{GoalID: ctx.GoalID, Complexity: "medium", SuggestedFlow: "generic-v1"}, nil
}
func (s *emptyStubAgent) Plan(criteria *missionengine.CompletionCriteria, analysis *missionengine.TaskAnalysis, flowName string, ctx missionengine.Context) (*missionengine.MissionGraph, error) {
	return &missionengine.MissionGraph{GoalID: ctx.GoalID}, nil
}
func (s *emptyStubAgent) PlanLegacy(goal string, ctx missionengine.Context) (*missionengine.MissionGraph, error) {
	return &missionengine.MissionGraph{GoalID: ctx.GoalID}, nil
}

func (s *emptyStubAgent) Verify(code string, actionID string, ctx missionengine.Context) (*missionengine.VerificationResult, error) {
	return &missionengine.VerificationResult{ActionID: actionID, Verdict: "PASS", Reason: "stub", Score: 100}, nil
}

func (s *cycleAgent) Verify(code string, actionID string, ctx missionengine.Context) (*missionengine.VerificationResult, error) {
	return &missionengine.VerificationResult{ActionID: actionID, Verdict: "PASS", Reason: "stub", Score: 100}, nil
}

func (s *badEdgeAgent) Verify(code string, actionID string, ctx missionengine.Context) (*missionengine.VerificationResult, error) {
	return &missionengine.VerificationResult{ActionID: actionID, Verdict: "PASS", Reason: "stub", Score: 100}, nil
}

func (s *selfLoopAgent) Verify(code string, actionID string, ctx missionengine.Context) (*missionengine.VerificationResult, error) {
	return &missionengine.VerificationResult{ActionID: actionID, Verdict: "PASS", Reason: "stub", Score: 100}, nil
}
