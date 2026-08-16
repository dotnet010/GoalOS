// Package scheduler — GoalRunner v0.1.0。
// Goal 级调度器。管理 Goal 生命周期（Draft→Running→Completed/Failed）。
// 处理暂停/恢复/终止。调用 PipelineRunner.Run(graph)。
// per-Goal 单线程控制环——每个 Goal 状态转换在单一 goroutine 中串行。
//
// 设计依据：05 架构文档 §3.1、R276。

package scheduler

import (
	"fmt"
	"log"
	"sync"
	"time"

	"github.com/goalos/goalos/internal/eventbus"
	"github.com/goalos/goalos/internal/statestore"
	"github.com/goalos/goalos/pkg/events"
)

// GoalRunner 管理单个 Goal 的生命周期。
// per-Goal 单线程控制环。并发控制指令通过 Event Bus 核心状态通道同步串行化。
type GoalRunner struct {
	goal           Goal
	bus            *eventbus.EventBus
	store          *statestore.Store
	pipelineRunner *PipelineRunner
	goalAnchor     *GoalAnchorTracker

	mu       sync.Mutex
	state    GoalStatus
	waitReason string // H16: "approval"|"dependency"|"resource"
}

// WaitReason 类型安全常量 (H16)。
type WaitReason string
const (
	WaitApproval   WaitReason = "approval"
	WaitDependency WaitReason = "dependency"
	WaitResource   WaitReason = "resource"
)

// Goal 是 GoalRunner 的输入。
type Goal struct {
	ID           string
	Title        string
	Description  string
	ArtifactPath string // A4: 产出物目录路径
	State        string // A4: 当前 Goal 状态
}

// NewGoalRunner 创建 GoalRunner。
func NewGoalRunner(goal Goal, bus *eventbus.EventBus, store *statestore.Store, pr *PipelineRunner, ga *GoalAnchorTracker) *GoalRunner {
	return &GoalRunner{
		goal:           goal,
		bus:            bus,
		store:          store,
		pipelineRunner: pr,
		goalAnchor:     ga,
		state:          StatusDraft,
	}
}

// Execute 是 GoalRunner 的主入口——per-Goal 单线程控制环。
// 阻塞直到 Goal 达到终态（Completed/Failed）。
func (gr *GoalRunner) Execute() error {
	gr.setState(StatusRunning)

	// G1: 循环直到终态（Completed/Failed），含 Paused 恢复
	for gr.state != StatusCompleted && gr.state != StatusFailed {
		// 加载最新 MissionGraph（可能因 REPLAN 而更新）
		state, err := gr.store.LoadState(gr.goal.ID)
		if err != nil {
			return fmt.Errorf("goalrunner: load state: %w", err)
		}

		// 调用 PipelineRunner 执行 Action 原语管线
		result, err := gr.pipelineRunner.Run(gr.goal.ID, state)
		if err != nil {
			return fmt.Errorf("goalrunner: pipeline: %w", err)
		}

		switch result.Status {
		case PipelineCompleted:
			gr.setState(StatusCompleted)
			gr.publishGoalCompleted()
			return nil

		case PipelineFailed:
			gr.setState(StatusFailed)
			gr.publishGoalFailed(result.Error)
			return nil

		case PipelineWaiting:
			// Wait 原语触发。保存 PipelineState 到 Snapshot
			gr.waitReason = result.WaitReason
			gr.savePipelineState(result.PipelineState)

			// 订阅唤醒事件并等待
			evt := gr.waitForWakeup(result)
			// G2: WaitTimeout→GoalFailed，不无限等待
			if evt.Type == "WaitTimeout" {
				gr.setState(StatusFailed)
				gr.publishGoalFailed("wait timeout: " + result.WaitReason)
				return nil
			}
			log.Printf("[GoalRunner] goal=%s woken by %s", gr.goal.ID, evt.Type)
			continue

		case PipelinePaused:
			// 用户暂停
			gr.setState(StatusPaused)
			gr.savePipelineState(result.PipelineState)
			gr.waitForResume()
			gr.setState(StatusRunning)
			continue
		}
	}
	return nil
}

// setState 更新 Goal 状态（线程安全）。
func (gr *GoalRunner) setState(s GoalStatus) {
	gr.mu.Lock()
	defer gr.mu.Unlock()
	gr.state = s
}

// State 返回当前 Goal 状态。
func (gr *GoalRunner) State() GoalStatus {
	gr.mu.Lock()
	defer gr.mu.Unlock()
	return gr.state
}

// savePipelineState 持久化 PipelineSnapshot 到 Snapshot。
// S'-12 改名映射表：执行位置快照唯一类型=statestore.PipelineSnapshot——
// 直接持久化（无跨包转换）。
func (gr *GoalRunner) savePipelineState(ps *statestore.PipelineSnapshot) error {
	state, err := gr.store.LoadState(gr.goal.ID)
	if err != nil {
		return fmt.Errorf("goalrunner: load state for snapshot: %w", err)
	}
	state.PipelineState = ps
	if err := gr.store.SaveSnapshot(gr.goal.ID, state); err != nil {
		return fmt.Errorf("goalrunner: save snapshot: %w", err)
	}
	// R-1376 双写契约可观测性：SaveSnapshot 以 LastAppliedSeq 命名快照文件，
	// 同一 seq 重写同名文件——消费方（契约测试/恢复路径）经 LoadLatestSnapshot
	// 读到的即是本次重写后的值。LastAppliedSeq 不在此递增（事件序号归
	// statestore.Append 唯一分配，R-1393）。
	log.Printf("[GoalRunner] goal=%s pipeline snapshot saved (wait_reason=%s timeout_at=%s)",
		gr.goal.ID, ps.WaitReason, ps.TimeoutAt)
	return nil
}

// waitForWakeup 等待外部唤醒事件。
// 订阅对应事件类型，阻塞直到事件到达或超时。
func (gr *GoalRunner) waitForWakeup(result *PipelineResult) events.Event {
	// R-1376 唤醒集闭合：订阅集并入拒绝族事件（非单一事件类型）
	eventTypes := wakeupEventSetForReason(result.WaitReason)
	ch := make(chan events.Event, 1)

	subIDs := make([]eventbus.SubscriptionID, 0, len(eventTypes))
	for _, eventType := range eventTypes {
		subID := gr.bus.SubscribeForGoal(gr.goal.ID, eventType, func(evt events.Event) error {
			ch <- evt
			return nil
		})
		subIDs = append(subIDs, subID)
	}
	defer func() {
		for _, subID := range subIDs {
			gr.bus.Unsubscribe(subID)
		}
	}()

	// 超时处理：Wait 不是永久阻塞。超时→返回 TimeoutEvent。
	// R-1376 唤醒集闭合：订阅必须建立后才允许超时计时生效——time.After 在 select
	// 求值时即创建计时器，若 goroutine 尚未完成 SubscribeForGoal 注册就已超时，
	// 后到的唤醒事件会丢失（已订阅但无人接收）。故先建立订阅，再以订阅完成时刻
	// 为基准计算剩余超时窗口。
	timeout := 5 * time.Minute
	if result.PipelineState != nil && result.PipelineState.TimeoutAt != "" {
		if t, err := time.Parse(time.RFC3339, result.PipelineState.TimeoutAt); err == nil {
			timeout = time.Until(t)
		}
	}
	if timeout < 0 {
		timeout = 0
	}

	select {
	case evt := <-ch:
		// R-1376/R-1379 wait_more 决策路径：UserDecisionReceived{decision:"wait_more"} 事件
		// 投递 GoalRunner——双重写=延长 governance 计时器+重写 PipelineState.TimeoutAt
		//（同一 approval_timeout 窗口）。
		if evt.Type == events.TypeUserDecisionReceived {
			// Event.Payload=map[string]interface{}（非 interface）——直接访问
			if decision, ok := evt.Payload["decision"].(string); ok && decision == "wait_more" {
				// 延长 PipelineState.TimeoutAt（同一 approval_timeout 窗口）
				if result.PipelineState != nil && result.PipelineState.TimeoutAt != "" {
					if t, err := time.Parse(time.RFC3339, result.PipelineState.TimeoutAt); err == nil {
						result.PipelineState.TimeoutAt = t.Add(gr.pipelineRunner.approvalTimeout).Format(time.RFC3339)
						// 双重写=延长+持久化（R-1376 双写契约——快照中 TimeoutAt 必须被重写）。
						// 持久化失败必须暴露——静默吞错违反 check-error-swallow，且 R-1376
						// "双写必须生效"要求失败可观测（写盘失败=契约未履行）。
						if err := gr.savePipelineState(result.PipelineState); err != nil {
							log.Printf("[GoalRunner] goal=%s wait_more snapshot persist failed: %v", gr.goal.ID, err)
						}
					}
				}
			}
		}
		return evt
	case <-time.After(timeout):
		log.Printf("[GoalRunner] goal=%s wait timeout after %v", gr.goal.ID, timeout)
		return events.Event{
			Type:   "WaitTimeout",
			GoalID: gr.goal.ID,
			Source: "goalrunner",
			Payload: map[string]interface{}{
				"wait_reason": gr.waitReason,
				"timeout":     timeout.String(),
			},
		}
	}
}

// waitForResume 等待用户恢复指令。
func (gr *GoalRunner) waitForResume() {
	ch := make(chan events.Event, 1)
	subID := gr.bus.SubscribeForGoal(gr.goal.ID, events.TypeGoalResumed, func(evt events.Event) error {
		ch <- evt
		return nil
	})
	defer gr.bus.Unsubscribe(subID)
	<-ch
}

// publishGoalCompleted 发布 GoalCompleted 事件。
// v0.2.2 W8 B15: Two-Phase Commit——Publish 前检查 events.jsonl 防止重复。
func (gr *GoalRunner) publishGoalCompleted() {
	// B15 Phase 1: 检查是否已发布（幂等保护）
	if state, err := gr.store.LoadState(gr.goal.ID); err == nil && state.InternalState == "completed" {
		log.Printf("[GoalRunner] GoalCompleted skipped: %s already completed in events.jsonl", gr.goal.ID)
		return
	}
	payload := GoalCompletedPayloadTyped{
		GoalID:       gr.goal.ID,
		ArtifactPath: gr.goal.ArtifactPath,
	}
	if err := payload.Validate(); err != nil {
		// G5: Validate 失败时回退到 map payload 确保事件不丢失
		log.Printf("[GoalRunner] GoalCompleted typed payload validation failed: %v — falling back to map payload", err)
		gr.bus.Publish(events.Event{
			Type:    events.TypeGoalCompleted,
			GoalID:  gr.goal.ID,
			Source:  "goalrunner",
			Payload: map[string]interface{}{"goal_id": gr.goal.ID, "artifact_path": gr.goal.ArtifactPath},
		})
		return
	}
	gr.bus.Publish(events.Event{
		Type:    events.TypeGoalCompleted,
		GoalID:  gr.goal.ID,
		Source:  "goalrunner",
		Payload: typedPayloadToMap(payload),
	})
}

// publishGoalFailed 发布 GoalFailed 事件（失败终态）。
// v0.2.2 W8 B15: Two-Phase Commit——Publish 前检查幂等。
func (gr *GoalRunner) publishGoalFailed(reason string) {
	// B15 Phase 2: 检查是否已发布（幂等保护）
	if state, err := gr.store.LoadState(gr.goal.ID); err == nil && state.InternalState == "failed" {
		log.Printf("[GoalRunner] GoalFailed skipped: %s already failed in events.jsonl", gr.goal.ID)
		return
	}
	payload := GoalFailedPayload{
		GoalID: gr.goal.ID,
		Reason: reason,
	}
	if err := payload.Validate(); err != nil {
		// G6: Validate 失败时回退到 map payload 确保事件不丢失
		log.Printf("[GoalRunner] GoalFailed typed payload validation failed: %v — falling back to map payload", err)
		gr.bus.Publish(events.Event{
			Type:    events.TypeGoalFailed,
			GoalID:  gr.goal.ID,
			Source:  "goalrunner",
			Payload: map[string]interface{}{"goal_id": gr.goal.ID, "reason": reason},
		})
		return
	}
	gr.bus.Publish(events.Event{
		Type:    events.TypeGoalFailed,
		GoalID:  gr.goal.ID,
		Source:  "goalrunner",
		Payload: typedPayloadToMap(payload),
	})
}

// wakeupEventSetForReason — 唤醒集闭合（R-1376——拒绝族入唤醒集）。
// 契约：waitForWakeup 订阅集并入拒绝族事件（ActionRejected/SecurityIncident/GoalNeedsReview/
// ActionCancelled）——拒绝族事件发布后 waitForWakeup 在超时前被唤醒（非静默超时）。
func wakeupEventSetForReason(reason string) []string {
	switch reason {
	case "approval":
		// 审批唤醒集=批准+拒绝族+wait_more 决策（R-1376 唤醒集闭合+R-1379 wait_more 投递）
		return []string{
			events.TypeUserApprovedAction,      // 批准
			events.TypeActionRejected,          // 拒绝（Governance 拒绝）
			events.TypeSecurityIncident,        // 安全事件（guard 族）
			events.TypeGoalNeedsReview,         // 需要审查（GoalNeedsReview）
			events.TypeActionCancelled,         // 取消（ActionCancelled）
			events.TypeUserDecisionReceived,    // wait_more 决策（R-1379——wait_more 经事件投递 GoalRunner）
		}
	case "dependency":
		return []string{events.TypeActionCompleted}
	case "resource":
		return []string{events.TypeResourceAvailable}
	default:
		return []string{events.TypeGoalResumed}
	}
}
