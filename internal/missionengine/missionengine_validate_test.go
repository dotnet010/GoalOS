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

func (a *cycleAgent) Plan(goal string, ctx missionengine.Context) (*missionengine.MissionGraph, error) {
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

// TestMissionGraph_InvalidEdgeRejected 验证边引用不存在的节点被拒绝。
func TestMissionGraph_InvalidEdgeRejected(t *testing.T) {
	bus := eventbus.New()
	eng := missionengine.New(bus, &badEdgeAgent{})
	eng.Start()

	rejected := make(chan events.Event, 1)
	bus.Subscribe(events.TypeMissionGraphRejected, func(evt events.Event) error {
		rejected <- evt
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
	case <-rejected:
		// ok — validation caught bad edge
	case <-time.After(time.Second):
		t.Fatal("边引用不存在的节点应触发 MissionGraphRejected")
	}
}

type badEdgeAgent struct{}

func (a *badEdgeAgent) Plan(goal string, ctx missionengine.Context) (*missionengine.MissionGraph, error) {
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

// TestMissionGraph_SelfLoopRejected 验证自循环边被拒绝。
func TestMissionGraph_SelfLoopRejected(t *testing.T) {
	bus := eventbus.New()
	eng := missionengine.New(bus, &selfLoopAgent{})
	eng.Start()

	rejected := make(chan events.Event, 1)
	bus.Subscribe(events.TypeMissionGraphRejected, func(evt events.Event) error {
		rejected <- evt
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
	case <-rejected:
		// ok
	case <-time.After(time.Second):
		t.Fatal("自循环边应触发 MissionGraphRejected")
	}
}

type selfLoopAgent struct{}

func (a *selfLoopAgent) Plan(goal string, ctx missionengine.Context) (*missionengine.MissionGraph, error) {
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
