// Package scheduler 实现 GoalOS Scheduler — 纯状态机驱动者。
// subscribe 核心事件驱动 Goal + Action 双状态机。
// transition() 是纯函数——在 transition.go 中。
// 设计依据：05 架构文档 §3、R153、R229。
package scheduler

import (
	"fmt"
	"log"
	"time"

	"github.com/goalos/goalos/internal/eventbus"
	"github.com/goalos/goalos/internal/statestore"
	"github.com/goalos/goalos/pkg/events"
)

// Scheduler 是 Goal 和 Action 状态机的唯一驱动者。
// 不包含业务逻辑。不直接调用 LLM/Agent/Plugin/Shell/Browser。
type Scheduler struct {
	bus              *eventbus.EventBus
	store            *statestore.Store
	goalAnchor       *GoalAnchorTracker
	completedActions map[string]int   // goalID → 已完成 Action 数
	totalActions     map[string]int   // goalID → 总 Action 数
	actionStates     map[string]ActionStatus // actionID → 当前 Action 状态
	verificationAttempts map[string]int // actionID → 验证重试次数 (max 3)
}

// New creates a Scheduler.
func New(bus *eventbus.EventBus, store *statestore.Store, goalAnchor *GoalAnchorTracker) *Scheduler {
	return &Scheduler{
		bus:                  bus,
		store:                store,
		goalAnchor:           goalAnchor,
		completedActions:     make(map[string]int),
		totalActions:         make(map[string]int),
		actionStates:         make(map[string]ActionStatus),
		verificationAttempts: make(map[string]int),
	}
}

// Start subscribes to core events and begins driving the state machine.
func (s *Scheduler) Start() {
	s.bus.Subscribe(events.TypeGoalCreated, s.handleGoalCreated)
	s.bus.Subscribe(events.TypeMissionGenerated, s.handleMissionGenerated)
	s.bus.Subscribe(events.TypeActionScheduled, s.handleActionScheduled)
	s.bus.Subscribe(events.TypeActionApproved, s.handleActionApproved)
	s.bus.Subscribe(events.TypeActionCompleted, s.handleActionCompleted)
	s.bus.Subscribe(events.TypeGoalCompleted, s.handleGoalCompleted)
	s.bus.Subscribe(events.TypeActionFailed, s.handleActionFailed)
	s.bus.Subscribe(events.TypeVerificationResult, s.handleVerificationResult)
	s.bus.Subscribe(events.TypeGoalPauseRequested, s.handlePauseRequested)
	s.bus.Subscribe(events.TypeGoalRollbackRequested, s.handleRollbackRequested)
	log.Println("[Scheduler] started, subscribed to state machine events")
}

func (s *Scheduler) handleActionFailed(evt events.Event) error {
	recoverable, _ := evt.Payload["recoverable"].(bool)
	actionID, _ := evt.Payload["action_id"].(string)
	errorType, _ := evt.Payload["error_type"].(string)
	log.Printf("[Scheduler] ActionFailed: %s (recoverable=%v, error_type=%s)", actionID, recoverable, errorType)

	// 更新 Action 状态
	s.actionStates[actionID] = ActionFailed

	// seccomp 违规 → 直接 HumanIntervention。不重试（安全事件必须人工审查）。
	if errorType == "seccomp_violation" {
		log.Printf("[Scheduler] seccomp violation: %s — direct HumanIntervention (no retry)", actionID)
		s.publish(events.Event{
			Type:   events.TypeHumanInterventionRequested,
			GoalID: evt.GoalID,
			Source: "scheduler",
			Payload: map[string]interface{}{
				"goal_id":           evt.GoalID,
				"failed_action_id":  actionID,
				"recovery_attempts": 0,
				"reason":            "seccomp_violation: 安全违规，必须人工审查",
			},
		})
		return nil
	}

	if recoverable {
		s.actionStates[actionID] = ActionRecovering
		// Recovery: Retry（指数退避由定时事件实现）
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

	// GoalAnchor: 每次 LLM 规划调用计数器+1。达阈值时注入 goal_anchor_check
	goalText, _ := evt.Payload["title"].(string)
	anchorCheck := s.goalAnchor.Increment(evt.GoalID)

	s.publish(events.Event{
		Type:   events.TypePlanRequested,
		GoalID: evt.GoalID,
		Source: "scheduler",
		Payload: map[string]interface{}{
			"goal_text":         goalText,
			"goal_anchor_check": anchorCheck,
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
	actionID, _ := evt.Payload["action_id"].(string)
	log.Printf("[Scheduler] ActionApproved: %s — PluginRunner will execute", actionID)
	s.actionStates[actionID] = ActionApproved
	// PluginRunner 订阅 ActionApproved → 启动子进程 → 发布 ActionCompleted
	return nil
}

// handleActionScheduled 记录 Action 被调度，追踪 Action 状态机。
func (s *Scheduler) handleActionScheduled(evt events.Event) error {
	actionID, _ := evt.Payload["action_id"].(string)
	s.actionStates[actionID] = ActionScheduled
	log.Printf("[Scheduler] ActionScheduled: %s state=Scheduled", actionID)
	return nil
}

// handleVerificationResult 处理验证结果（验证循环核心）。
func (s *Scheduler) handleVerificationResult(evt events.Event) error {
	actionID, _ := evt.Payload["action_id"].(string)
	status, _ := evt.Payload["status"].(string)
	log.Printf("[Scheduler] VerificationResult: %s status=%s", actionID, status)

	if status == "verified" {
		s.actionStates[actionID] = ActionCompleted
		return nil
	}

	// FAIL: 自修正逻辑
	s.actionStates[actionID] = ActionVerifying
	attempts := s.verificationAttempts[actionID] + 1
	s.verificationAttempts[actionID] = attempts

	if attempts > 3 {
		s.publish(events.Event{
			Type:   events.TypeSelfCorrectionExhausted,
			GoalID: evt.GoalID,
			Source: "scheduler",
			Payload: map[string]interface{}{
				"action_id":  actionID,
				"attempts":   attempts,
				"last_diff":  evt.Payload["diff"],
			},
		})
		return nil
	}

	s.publish(events.Event{
		Type:   events.TypeVerificationFailed,
		GoalID: evt.GoalID,
		Source: "scheduler",
		Payload: map[string]interface{}{
			"action_id":           actionID,
			"verification_method": evt.Payload["method"],
			"expected":            evt.Payload["expected"],
			"actual":              evt.Payload["actual"],
			"diff":                evt.Payload["diff"],
			"attempt":             attempts,
		},
	})
	return nil
}

// handlePauseRequested 处理用户暂停请求（异步审批竞态处理）。
func (s *Scheduler) handlePauseRequested(evt events.Event) error {
	log.Printf("[Scheduler] PauseRequested: %s — cancelling pending approvals", evt.GoalID)
	// 发布 ActionCancelled 取消所有 pending 审批
	for actionID, state := range s.actionStates {
		if state == ActionScheduled || state == ActionApproved {
			s.publish(events.Event{
				Type:   events.TypeActionCancelled,
				GoalID: evt.GoalID,
				Source: "scheduler",
				Payload: map[string]interface{}{
					"action_id": actionID,
					"reason":    "user_paused",
				},
			})
		}
	}
	return nil
}

// handleRollbackRequested 处理用户回滚请求（异步审批竞态处理）。
func (s *Scheduler) handleRollbackRequested(evt events.Event) error {
	log.Printf("[Scheduler] RollbackRequested: %s — cancelling all pending actions", evt.GoalID)
	// 回滚取消所有并行 pending 审批
	for actionID, state := range s.actionStates {
		if state == ActionScheduled || state == ActionApproved {
			s.publish(events.Event{
				Type:   events.TypeActionCancelled,
				GoalID: evt.GoalID,
				Source: "scheduler",
				Payload: map[string]interface{}{
					"action_id": actionID,
					"reason":    "user_rollback",
				},
			})
		}
	}
	return nil
}

func (s *Scheduler) handleActionCompleted(evt events.Event) error {
	actionID, _ := evt.Payload["action_id"].(string)
	s.actionStates[actionID] = ActionCompleted
	s.completedActions[evt.GoalID]++
	total := s.totalActions[evt.GoalID]
	if total > 0 && s.completedActions[evt.GoalID] >= total {
		log.Printf("[Scheduler] GoalCompleted: %s (all %d actions done)", evt.GoalID, total)
		s.publish(events.Event{Type: events.TypeGoalCompleted, GoalID: evt.GoalID, Source: "scheduler",
			Payload: map[string]interface{}{
				"completed_at":    fmt.Sprintf("%d", time.Now().Unix()),
				"artifact_path":   fmt.Sprintf("~/Goals/%s/", evt.GoalID),
				"total_actions":   total,
				"duration_seconds": 0,
				"total_tokens":    0,
				"human_interventions": 0,
			},
		})
	}
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
