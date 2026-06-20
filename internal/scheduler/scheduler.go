// Package scheduler 实现 GoalOS Scheduler — W1 骨架。
// 纯状态机驱动者。Subscribe 5 个核心事件驱动 Goal 状态机。
// transition() 是纯函数——在 transition.go 中。
// 设计依据：05 架构文档 §3、R153、R229。
package scheduler

import (
	"fmt"
	"log"

	"github.com/goalos/goalos/internal/eventbus"
	"github.com/goalos/goalos/internal/statestore"
	"github.com/goalos/goalos/pkg/events"
)

// Scheduler is the W1 Skeleton Scheduler.
// 设计依据：05 架构文档 §3、R153、R229。
type Scheduler struct {
	bus              *eventbus.EventBus
	store            *statestore.Store
	completedActions map[string]int // goalID → 已完成 Action 数
	totalActions     map[string]int // goalID → 总 Action 数
}

// New creates a Scheduler.
func New(bus *eventbus.EventBus, store *statestore.Store) *Scheduler {
	return &Scheduler{
		bus:              bus,
		store:            store,
		completedActions: make(map[string]int),
		totalActions:     make(map[string]int),
	}
}

// Start subscribes to core events and begins driving the state machine.
func (s *Scheduler) Start() {
	s.bus.Subscribe(events.TypeGoalCreated, s.handleGoalCreated)
	s.bus.Subscribe(events.TypeMissionGenerated, s.handleMissionGenerated)
	s.bus.Subscribe(events.TypeActionApproved, s.handleActionApproved)
	s.bus.Subscribe(events.TypeActionCompleted, s.handleActionCompleted)
	s.bus.Subscribe(events.TypeGoalCompleted, s.handleGoalCompleted)
	s.bus.Subscribe(events.TypeActionFailed, s.handleActionFailed)
	log.Println("[Scheduler] started, subscribed to state machine events")
}

func (s *Scheduler) handleActionFailed(evt events.Event) error {
	recoverable, _ := evt.Payload["recoverable"].(bool)
	actionID, _ := evt.Payload["action_id"].(string)
	log.Printf("[Scheduler] ActionFailed: %s (recoverable=%v)", actionID, recoverable)

	if recoverable {
		// Recovery: 发布 ActionRetrying
		s.publish(events.Event{
			Type:   events.TypeActionRetrying,
			GoalID: evt.GoalID,
			Source: "scheduler",
			Payload: map[string]interface{}{
				"action_id":       actionID,
				"attempt":         1,
				"backoff_seconds": 1,
			},
		})
	} else {
		// 不可恢复 → 请求人工介入
		s.publish(events.Event{
			Type:   events.TypeHumanInterventionRequested,
			GoalID: evt.GoalID,
			Source: "scheduler",
			Payload: map[string]interface{}{
				"goal_id":           evt.GoalID,
				"failed_action_id":  actionID,
				"recovery_attempts": 0,
				"reason":            "不可恢复的 Action 失败",
			},
		})
	}
	return nil
}

func (s *Scheduler) handleGoalCreated(evt events.Event) error {
	log.Printf("[Scheduler] GoalCreated: %s", evt.GoalID)

	// GoalCreated → always emit PlanRequested to trigger Mission Engine
	// Transition Draft→Planned happens on MissionGenerated, not GoalCreated
	goalText, _ := evt.Payload["title"].(string)
	s.publish(events.Event{
		Type:   events.TypePlanRequested,
		GoalID: evt.GoalID,
		Source: "scheduler",
		Payload: map[string]interface{}{
			"goal_text":         goalText,
			"goal_anchor_check": false,
		},
	})
	return nil
}

func (s *Scheduler) handleMissionGenerated(evt events.Event) error {
	nodeCount, _ := evt.Payload["node_count"].(float64); s.totalActions[evt.GoalID] = int(nodeCount)
	log.Printf("[Scheduler] MissionGenerated: %s (nodes=%d)", evt.GoalID, int(nodeCount))

	// W1: auto-confirm. Transition Planned → Running
	s.publish(events.Event{
		Type:   events.TypeUserConfirmed,
		GoalID: evt.GoalID,
		Source: "scheduler",
	})

	// Emit ActionScheduled for each node
	for i := 0; i < int(nodeCount); i++ {
		s.publish(events.Event{
			Type:   events.TypeActionScheduled,
			GoalID: evt.GoalID,
			Source: "scheduler",
			Payload: map[string]interface{}{
				"action_id":             generateActionID(evt.GoalID, i+1),
				"action_type":           "fs.read",
				"target":                "w1-stub",
				"required_capabilities": []interface{}{"fs.read"},
				"timeout_seconds":       30,
				"risk_level_pre":        "L0",
			},
		})
	}
	return nil
}

func (s *Scheduler) handleActionApproved(evt events.Event) error {
	log.Printf("[Scheduler] ActionApproved: %s — PluginRunner will execute", evt.GoalID)
	// PluginRunner 订阅 ActionApproved → 启动子进程 → 发布 ActionCompleted
	// Scheduler 不再伪造执行结果
	return nil
}

func (s *Scheduler) handleActionCompleted(evt events.Event) error {
	// W1: after all actions, auto-complete goal
	// In W3+, Scheduler tracks action count and emits GoalCompleted when all done
	s.completedActions[evt.GoalID]++; total := s.totalActions[evt.GoalID]; if total > 0 && s.completedActions[evt.GoalID] >= total { log.Printf("[Scheduler] GoalCompleted: %s", evt.GoalID); s.publish(events.Event{Type: events.TypeGoalCompleted, GoalID: evt.GoalID, Source: "scheduler"}); }
	return nil
}

func (s *Scheduler) handleGoalCompleted(evt events.Event) error {
	log.Printf("[Scheduler] GoalCompleted: %s — W1 chain complete!", evt.GoalID)
	return nil
}

func (s *Scheduler) publish(evt events.Event) {
	s.bus.Publish(evt)
}

var actionCounter int

func generateActionID(goalID string, idx int) string {
	actionCounter++
	return fmt.Sprintf("%s_act_%02d", goalID, idx)
}
