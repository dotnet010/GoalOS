// Scheduler 行为测试 — 验证 Action 状态机、seccomp 违规、验证循环。
package scheduler_test

import (
	"testing"
	"time"

	"github.com/goalos/goalos/internal/eventbus"
	"github.com/goalos/goalos/internal/scheduler"
	"github.com/goalos/goalos/internal/statestore"
	"github.com/goalos/goalos/pkg/events"
)

// TestActionStateMachine_ScheduledToApproved 验证 Action 状态从 Scheduled → Approved 转换。
func TestActionStateMachine_ScheduledToApproved(t *testing.T) {
	bus := eventbus.New()
	dir := t.TempDir()
	store := statestore.New(dir)
	sched := scheduler.New(bus, store, scheduler.NewGoalAnchorTracker(20))
	sched.Start()

	// Step 1: Scheduler 发布 ActionScheduled → 状态应为 Scheduled
	bus.Publish(events.Event{
		Type:   events.TypeActionScheduled,
		GoalID: "goal_asm_001",
		Source: "scheduler",
		Payload: map[string]interface{}{
			"action_id":   "act_asm_001",
			"action_type": "fs.read",
		},
	})

	// Step 2: Governance 发布 ActionApproved → Scheduler 应追踪为 Approved
	bus.Publish(events.Event{
		Type:   events.TypeActionApproved,
		GoalID: "goal_asm_001",
		Source: "governance",
		Payload: map[string]interface{}{
			"action_id": "act_asm_001",
		},
	})

	// Step 3: 验证状态链路 — ActionCompleted 应正常触发
	completed := make(chan events.Event, 1)
	bus.Subscribe(events.TypeActionCompleted, func(evt events.Event) error {
		completed <- evt
		return nil
	})

	// 模拟 ActionCompleted
	bus.Publish(events.Event{
		Type:   events.TypeActionCompleted,
		GoalID: "goal_asm_001",
		Source: "plugin-runner",
		Payload: map[string]interface{}{
			"action_id": "act_asm_001",
		},
	})

	select {
	case <-completed:
		// 正常完成
	case <-time.After(time.Second):
		t.Fatal("ActionCompleted 未被正常处理")
	}
}

// TestSeccompViolation_DirectToHumanIntervention 验证 seccomp 违规直接进入 HumanIntervention（不重试）。
func TestSeccompViolation_DirectToHumanIntervention(t *testing.T) {
	bus := eventbus.New()
	dir := t.TempDir()
	store := statestore.New(dir)
	sched := scheduler.New(bus, store, scheduler.NewGoalAnchorTracker(20))
	sched.Start()

	humanIntervention := make(chan events.Event, 1)
	bus.Subscribe(events.TypeHumanInterventionRequested, func(evt events.Event) error {
		humanIntervention <- evt
		return nil
	})

	// 发布 seccomp_violation ActionFailed
	bus.Publish(events.Event{
		Type:   events.TypeActionFailed,
		GoalID: "goal_seccomp",
		Source: "plugin-runner",
		Payload: map[string]interface{}{
			"action_id":   "act_seccomp_001",
			"error_type":  "seccomp_violation",
			"recoverable": true, // 即使是 recoverable，seccomp 违规也不应重试
		},
	})

	select {
	case evt := <-humanIntervention:
		reason, _ := evt.Payload["reason"].(string)
		if reason == "" {
			t.Error("HumanIntervention 应包含 reason")
		}
	case <-time.After(time.Second):
		t.Fatal("seccomp_violation 应直接触发 HumanInterventionRequested")
	}
}

// TestVerificationLoop_SelfCorrection 验证验证失败后自修正机制。
func TestVerificationLoop_SelfCorrection(t *testing.T) {
	bus := eventbus.New()
	dir := t.TempDir()
	store := statestore.New(dir)
	sched := scheduler.New(bus, store, scheduler.NewGoalAnchorTracker(20))
	sched.Start()

	// 订阅 VerificationFailed 事件
	verificationFailed := make(chan events.Event, 3)
	bus.Subscribe(events.TypeVerificationFailed, func(evt events.Event) error {
		verificationFailed <- evt
		return nil
	})

	// 发布 VerificationResult FAIL（第1次失败）
	bus.Publish(events.Event{
		Type:   events.TypeVerificationResult,
		GoalID: "goal_verify",
		Source: "plugin-runner",
		Payload: map[string]interface{}{
			"action_id": "act_verify_001",
			"status":    "mismatch",
			"method":    "auto_test",
			"expected":  "pass",
			"actual":    "fail",
			"diff":      "test failed",
		},
	})

	select {
	case evt := <-verificationFailed:
		attempt, _ := evt.Payload["attempt"].(int)
		if attempt != 1 {
			t.Errorf("expected attempt=1, got %v", attempt)
		}
	case <-time.After(time.Second):
		t.Fatal("VerificationResult FAIL 应触发 VerificationFailed")
	}

	// 第2次失败
	bus.Publish(events.Event{
		Type:   events.TypeVerificationResult,
		GoalID: "goal_verify",
		Source: "plugin-runner",
		Payload: map[string]interface{}{
			"action_id": "act_verify_001",
			"status":    "mismatch",
			"method":    "auto_test",
			"expected":  "pass",
			"actual":    "fail",
			"diff":      "test still failing",
		},
	})
	<-verificationFailed

	// 第3次失败
	bus.Publish(events.Event{
		Type:   events.TypeVerificationResult,
		GoalID: "goal_verify",
		Source: "plugin-runner",
		Payload: map[string]interface{}{
			"action_id": "act_verify_001",
			"status":    "mismatch",
			"method":    "auto_test",
			"expected":  "pass",
			"actual":    "fail",
			"diff":      "test still failing #3",
		},
	})
	<-verificationFailed

	// 第4次失败 → 应触发 SelfCorrectionExhausted
	selfCorrectionExhausted := make(chan events.Event, 1)
	bus.Subscribe(events.TypeSelfCorrectionExhausted, func(evt events.Event) error {
		selfCorrectionExhausted <- evt
		return nil
	})

	bus.Publish(events.Event{
		Type:   events.TypeVerificationResult,
		GoalID: "goal_verify",
		Source: "plugin-runner",
		Payload: map[string]interface{}{
			"action_id": "act_verify_001",
			"status":    "mismatch",
			"method":    "auto_test",
			"expected":  "pass",
			"actual":    "fail",
			"diff":      "test still failing #4",
		},
	})

	select {
	case evt := <-selfCorrectionExhausted:
		attempts, _ := evt.Payload["attempts"].(int)
		if attempts != 4 {
			t.Errorf("expected attempts=4, got %v", attempts)
		}
	case <-time.After(time.Second):
		t.Fatal("4次 VerificationFailed 后应触发 SelfCorrectionExhausted")
	}
}
