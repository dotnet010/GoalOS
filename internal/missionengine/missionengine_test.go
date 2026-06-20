package missionengine_test

import (
	"testing"

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

	// Simulate Scheduler publishing PlanRequested
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
		if int(nodeCount) != 3 {
			t.Errorf("expected 3 nodes, got %d", int(nodeCount))
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
		// Expected
	default:
		t.Fatal("MissionGraphRejected was not published for empty graph")
	}
}

// emptyStubAgent returns an empty MissionGraph to test validation.
type emptyStubAgent struct{}

func (s *emptyStubAgent) Plan(goal string, ctx missionengine.Context) (*missionengine.MissionGraph, error) {
	return &missionengine.MissionGraph{}, nil
}
