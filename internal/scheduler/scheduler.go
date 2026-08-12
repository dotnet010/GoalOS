// Package scheduler 实现 GoalOS Scheduler — 纯状态机驱动者。
// subscribe 核心事件驱动 Goal + Action 双状态机。
// transition() 是纯函数——在 transition.go 中。
// 设计依据：05 架构文档 §3、R153、R229。
package scheduler

import (
	"fmt"
	"log"
	"sync"
	"sync/atomic"
	"time"

	"github.com/goalos/goalos/internal/eventbus"
	"github.com/goalos/goalos/internal/statestore"
	"github.com/goalos/goalos/pkg/events"
)

// execTimeoutSec 是 Action 执行超时秒数（R-1059）。
// 与审批超时（policy.approval_timeout）、令牌 TTL（policy.token_ttl）三值独立——
// 此前为裸魔法数 30，且被 governance 误读为令牌 TTL 计算基数（2×30=60s 偶然耦合）。
const execTimeoutSec = 30

// Scheduler 是 Goal 和 Action 状态机的唯一驱动者。
type Scheduler struct {
	bus              *eventbus.EventBus
	store            *statestore.Store
	goalAnchor       *GoalAnchorTracker
	autonomyLevel    string
	goalTexts        map[string]string // v0.1.1 fix: goalID→goalText for GoalCompleted payload
	mu               sync.Mutex
	completedActions map[string]int
	failedActions    map[string]int
	totalActions     map[string]int
	actionStates     map[string]ActionStatus
	verificationAttempts map[string]int
	goalTimers       map[string]*time.Timer
	goalProgressed   map[string]bool
	goalFailed       map[string]bool // v0.2.2 W5 A22: GoalFailed 标记
	scheduling       map[string]bool // R-839: 调度中标志——阻止提前GoalCompleted
}

// SetAutonomyLevel sets autonomy level（v0.1.1）。
// R-1058: mu 保护——热重载（handleConfigReloaded）与读取并发。
func (s *Scheduler) SetAutonomyLevel(level string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.autonomyLevel = level
}

// New creates a Scheduler.
func New(bus *eventbus.EventBus, store *statestore.Store, goalAnchor *GoalAnchorTracker) *Scheduler {
	return &Scheduler{
		bus:                  bus,
		store:                store,
		goalAnchor:           goalAnchor,
		goalTexts:            make(map[string]string),
		completedActions:     make(map[string]int),
		failedActions:        make(map[string]int),
		totalActions:         make(map[string]int),
		actionStates:         make(map[string]ActionStatus),
		verificationAttempts: make(map[string]int),
		goalTimers:           make(map[string]*time.Timer),
		goalProgressed:       make(map[string]bool),
		goalFailed:           make(map[string]bool),
		scheduling:           make(map[string]bool), // R-839
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
	s.bus.Subscribe(events.TypeConfigReloaded, s.handleConfigReloaded) // R-1058: 热重载参数经事件总线进入
	log.Println("[Scheduler] started, subscribed to state machine events")
}

// handleConfigReloaded 应用热重载后的调度参数（R-1058）。
// 只影响"新一代"决策；进行中 Goal 的状态机照旧运行。
func (s *Scheduler) handleConfigReloaded(evt events.Event) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if v, ok := evt.Payload["autonomy_level"].(string); ok && v != "" {
		s.autonomyLevel = v
		log.Printf("[Scheduler] config reloaded: autonomy=%s", v)
	}
	return nil
}

func (s *Scheduler) handleActionFailed(evt events.Event) error {
	actionID, ok := evt.Payload["action_id"].(string)
	if !ok || actionID == "" {
		return fmt.Errorf("scheduler: ActionFailed missing action_id")
	}
	recoverable, _ := evt.Payload["recoverable"].(bool)
	errorType, _ := evt.Payload["error_type"].(string)
	log.Printf("[Scheduler] ActionFailed: %s (recoverable=%v, error_type=%s)", actionID, recoverable, errorType)

	s.mu.Lock()
	s.actionStates[actionID] = ActionFailed
	if !recoverable {
		s.failedActions[evt.GoalID]++
	}
	total := s.totalActions[evt.GoalID]
	doneOrFailed := s.completedActions[evt.GoalID] + s.failedActions[evt.GoalID]
	allResolved := total > 0 && doneOrFailed >= total
	succeeded := s.completedActions[evt.GoalID]
	// W5-A22 fix: 在锁内设置 goalFailed，防止 data race
	if allResolved && succeeded == 0 {
		s.goalFailed[evt.GoalID] = true
	}
	s.mu.Unlock()

	if allResolved {
		if succeeded > 0 {
			log.Printf("[Scheduler] GoalCompleted: %s (%d/%d succeeded)", evt.GoalID, succeeded, total)
			s.publish(events.Event{
				Type:    events.TypeGoalCompleted,
			GoalID:  evt.GoalID,
			Source:  "scheduler",
			Payload: map[string]interface{}{
				"reason": fmt.Sprintf("%d/%d actions succeeded", succeeded, total),
					"failed": float64(s.failedActions[evt.GoalID]),
				},
			})
		} else {
			// W5-A22 fix: goalFailed 必须在锁内设置，防止 data race
			log.Printf("[Scheduler] GoalFailed: %s (all %d actions failed)", evt.GoalID, total)
			s.publish(events.Event{
				Type:    events.TypeGoalFailed,
				GoalID:  evt.GoalID,
				Source:  "scheduler",
				Payload: map[string]interface{}{
					"reason": fmt.Sprintf("all %d actions failed, 0 succeeded", total),
					"error":  "no output produced",
				},
			})
		}
		// 取消超时计时器
		if t, ok := s.goalTimers[evt.GoalID]; ok { t.Stop(); delete(s.goalTimers, evt.GoalID) }
		return nil
	}

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
		s.mu.Lock()
		s.actionStates[actionID] = ActionRecovering
		s.mu.Unlock()
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

	// 1800s 规划阶段超时（v0.2.2 W5 A24: 注释修正——代码实际为 1800s）
	s.mu.Lock()
	s.goalProgressed[evt.GoalID] = false
	s.goalTimers[evt.GoalID] = time.AfterFunc(1800*time.Second, func() { // 1800s 规划阶段超时
		s.mu.Lock()
		progressed := s.goalProgressed[evt.GoalID]
		if !progressed {
			s.goalFailed[evt.GoalID] = true
		}
		s.mu.Unlock()
		if !progressed {
			log.Printf("[Scheduler] Goal %s: 1800s timeout — no action progress, marking failed", evt.GoalID)
			s.publish(events.Event{
				Type:   events.TypeGoalFailed,
				GoalID: evt.GoalID,
				Source: "scheduler",
				Payload: map[string]interface{}{
					"reason": "timeout: no action progress",
					"error": "goal timed out",
				},
			})
		}
	})
	s.mu.Unlock()

	// GoalAnchor: 每次 LLM 规划调用计数器+1。达阈值时注入 goal_anchor_check
	goalText, _ := evt.Payload["title"].(string)
	s.goalTexts[evt.GoalID] = goalText // v0.1.1 fix: 供 GoalCompleted payload 使用
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
	s.mu.Lock()
	// H9: 仅首次初始化，REPLAN 时不重置（保留旧计划统计）
	if _, ok := s.totalActions[evt.GoalID]; !ok {
		s.totalActions[evt.GoalID] = 0
	}
	// 规划完成→重置超时计时器（为执行阶段提供 60s）
	s.goalProgressed[evt.GoalID] = false
	if old, ok := s.goalTimers[evt.GoalID]; ok { old.Stop() }
	s.goalTimers[evt.GoalID] = time.AfterFunc(60*time.Second, func() {
		s.mu.Lock()
		progressed := s.goalProgressed[evt.GoalID]
		s.mu.Unlock()
		if !progressed {
			log.Printf("[Scheduler] Goal %s: 60s execution timeout", evt.GoalID)
			s.publish(events.Event{
				Type:   events.TypeGoalFailed,
				GoalID: evt.GoalID,
				Source: "scheduler",
				Payload: map[string]interface{}{
					"reason": "timeout: execution timeout",
					"error": "execution timed out",
				},
			})
		}
	})
	s.mu.Unlock()

	// 读取 MissionGraph node 列表
	nodesRaw, _ := evt.Payload["nodes"].([]interface{})
	nodeCount := len(nodesRaw)
	log.Printf("[Scheduler] MissionGenerated: %s (nodes=%d)", evt.GoalID, nodeCount)
	if nodeCount == 0 {
		// 向后兼容：无 nodes 字段时 fallback 到 node_count
		nc, _ := evt.Payload["node_count"].(float64)
		nodeCount = int(nc)
	}

	// v0.1.1: autonomous/full 模式下自动确认，否则等待用户通过 Channel Adapter 确认。
	// R-1058: 读锁——热重载写入经 handleConfigReloaded 同锁保护。
	s.mu.Lock()
	autonomy := s.autonomyLevel
	s.mu.Unlock()
	if autonomy == "autonomous" || autonomy == "full" {
		s.publish(events.Event{
			Type:   events.TypeUserConfirmed,
			GoalID: evt.GoalID,
			Source: "scheduler",
		})
	}

	// R-839: 调度开始——阻止 handleActionCompleted 提前触发 GoalCompleted
	s.mu.Lock()
	s.scheduling[evt.GoalID] = true
	s.mu.Unlock()

	// R-839: 收集 action IDs 写入 GoalState
	var nodeIDs []string
	for i := 0; i < nodeCount; i++ {
		nodeIDs = append(nodeIDs, generateActionID(evt.GoalID, i+1))
	}

	// R-839: 写入 GoalState.NodeIDs——GoalRunner 需要知道所有节点
	if state, err := s.store.LoadState(evt.GoalID); err == nil {
		state.NodeIDs = nodeIDs
		s.store.SaveState(evt.GoalID, state)
	}

	// 按节点生成 ActionScheduled
	for i := 0; i < nodeCount; i++ {
		actionType := "fs.read"
		target := "generic"
		if i < len(nodesRaw) {
			if node, ok := nodesRaw[i].(map[string]interface{}); ok {
				if at, ok := node["action_type"].(string); ok && at != "" {
					actionType = at
				}
				if t, ok := node["target"].(string); ok && t != "" {
					target = t
				}
			}
		}
		// 按 action_type 推算风险等级
		riskLevel := "L0"
		if actionType == "web.search" {
			riskLevel = "L1"
		}
		s.mu.Lock()
		s.totalActions[evt.GoalID]++
		t := s.totalActions[evt.GoalID]
		s.mu.Unlock()
		log.Printf("[Scheduler] set totalActions[%s]=%d", evt.GoalID, t)
		s.publish(events.Event{
			Type:   events.TypeActionScheduled,
			GoalID: evt.GoalID,
			Source: "scheduler",
			Payload: map[string]interface{}{
				"action_id":             generateActionID(evt.GoalID, i+1),
				"action_type":           actionType,
				"target":                target,
				"required_capabilities": []interface{}{actionType},
				"timeout_seconds":       float64(execTimeoutSec), // R-1059: 命名常量，语义=执行超时
				"risk_level_pre":        riskLevel,
			},
		})
	}
	// R-839: 调度完成——允许 handleActionCompleted 检查 allDone
	s.mu.Lock()
	delete(s.scheduling, evt.GoalID)
	s.mu.Unlock()
	return nil
}

func (s *Scheduler) handleActionApproved(evt events.Event) error {
	actionID, ok := evt.Payload["action_id"].(string)
	if !ok || actionID == "" {
		return fmt.Errorf("scheduler: ActionApproved missing action_id")
	}
	log.Printf("[Scheduler] ActionApproved: %s — PluginRunner will execute", actionID)
	s.mu.Lock()
	s.actionStates[actionID] = ActionApproved
	s.mu.Unlock()
	return nil
}

func (s *Scheduler) handleActionScheduled(evt events.Event) error {
	actionID, ok := evt.Payload["action_id"].(string)
	if !ok || actionID == "" {
		return fmt.Errorf("scheduler: ActionScheduled missing action_id")
	}
	s.mu.Lock()
	s.actionStates[actionID] = ActionScheduled
	s.goalProgressed[evt.GoalID] = true // 标记进展，取消超时
	if t, ok := s.goalTimers[evt.GoalID]; ok { t.Stop(); delete(s.goalTimers, evt.GoalID) }
	s.mu.Unlock()
	log.Printf("[Scheduler] ActionScheduled: %s state=Scheduled", actionID)
	return nil
}

func (s *Scheduler) handleVerificationResult(evt events.Event) error {
	actionID, ok := evt.Payload["action_id"].(string)
	if !ok || actionID == "" {
		return fmt.Errorf("scheduler: VerificationResult missing action_id")
	}
	status, _ := evt.Payload["status"].(string)
	log.Printf("[Scheduler] VerificationResult: %s status=%s", actionID, status)

	if status == "verified" {
		s.mu.Lock()
		s.actionStates[actionID] = ActionCompleted
		s.mu.Unlock()
		return nil
	}

	s.mu.Lock()
	s.actionStates[actionID] = ActionVerifying
	attempts := s.verificationAttempts[actionID] + 1
	s.verificationAttempts[actionID] = attempts
	s.mu.Unlock()

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
	// 锁定后收集待取消的 actionID，解锁后再 publish（避免 publish 触发同 Scheduler handler 导致死锁）
	s.mu.Lock()
	var toCancel []string
	for actionID, state := range s.actionStates {
		if state == ActionScheduled || state == ActionApproved {
			toCancel = append(toCancel, actionID)
		}
	}
	s.mu.Unlock()
	for _, actionID := range toCancel {
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
	return nil
}

func (s *Scheduler) handleRollbackRequested(evt events.Event) error {
	log.Printf("[Scheduler] RollbackRequested: %s — cancelling all pending actions", evt.GoalID)
	s.mu.Lock()
	var toCancel []string
	for actionID, state := range s.actionStates {
		if state == ActionScheduled || state == ActionApproved {
			toCancel = append(toCancel, actionID)
		}
	}
	s.mu.Unlock()
	for _, actionID := range toCancel {
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
	return nil
}

func (s *Scheduler) handleActionCompleted(evt events.Event) error {
	actionID, ok := evt.Payload["action_id"].(string)
	if !ok || actionID == "" {
		return fmt.Errorf("scheduler: ActionCompleted missing action_id")
	}
	log.Printf("[Scheduler] ActionCompleted received: %s", actionID)
	s.mu.Lock()
	// R-839: 调度中——不检查 allDone（等循环结束）
	if s.scheduling[evt.GoalID] {
		s.actionStates[actionID] = ActionCompleted
		s.completedActions[evt.GoalID]++
		s.mu.Unlock()
		return nil
	}
	s.actionStates[actionID] = ActionCompleted
	s.completedActions[evt.GoalID]++
	total := s.totalActions[evt.GoalID]
	completed := s.completedActions[evt.GoalID]
	allDone := total > 0 && completed >= total
	s.mu.Unlock()
	log.Printf("[Scheduler] ActionCompleted counts: goal=%s completed=%d total=%d allDone=%v", evt.GoalID, completed, total, allDone)

	if allDone {
		log.Printf("[Scheduler] GoalCompleted: %s (all %d actions done)", evt.GoalID, total)
		// R-828: typed payload
		payload := GoalCompletedPayloadTyped{
			GoalID: evt.GoalID, ArtifactPath: fmt.Sprintf("~/Goals/%s/", evt.GoalID),
			TotalActions: total,
		}
		s.publish(events.Event{Type: events.TypeGoalCompleted, GoalID: evt.GoalID, Source: "scheduler",
			Payload: typedPayloadToMap(payload),
		})
	}
	return nil
}

func (s *Scheduler) handleGoalCompleted(evt events.Event) error {
	// v0.2.2 W5 (A22): 防止 GoalFailed 后重复触发 GoalCompleted
	s.mu.Lock()
	failed := s.goalFailed[evt.GoalID]
	s.mu.Unlock()
	if failed {
		log.Printf("[Scheduler] GoalCompleted: %s — skipped (already marked Failed)", evt.GoalID)
		return nil
	}
	// W5-A22 fix: cleanup——Goal 终态后清理 maps 防止内存泄漏
	s.mu.Lock()
	delete(s.goalFailed, evt.GoalID)
	delete(s.goalProgressed, evt.GoalID)
	delete(s.totalActions, evt.GoalID)
	delete(s.completedActions, evt.GoalID)
	delete(s.failedActions, evt.GoalID)
	s.mu.Unlock()
	log.Printf("[Scheduler] GoalCompleted: %s — W1 chain complete!", evt.GoalID)
	return nil
}

func (s *Scheduler) publish(evt events.Event) {
	s.bus.Publish(evt)
}

var actionCounter atomic.Int64

func generateActionID(goalID string, idx int) string {
	actionCounter.Add(1)
	return fmt.Sprintf("%s_act_%02d", goalID, idx)
}
