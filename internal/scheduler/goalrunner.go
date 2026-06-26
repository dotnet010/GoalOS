// Package scheduler — GoalRunner v1.1.0。
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
	waitReason string // v1.1.0: "approval"|"dependency"|"resource"。Running 子状态
}

// Goal 是 GoalRunner 的输入。
type Goal struct {
	ID          string
	Title       string
	Description string
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

	for gr.state == StatusRunning {
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
			log.Printf("[GoalRunner] goal=%s woken by %s", gr.goal.ID, evt.Type)

			// 恢复：重新进入循环。PipelineRunner 从 ResumePrimitive 继续
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

// savePipelineState 持久化 PipelineState 到 Snapshot。
func (gr *GoalRunner) savePipelineState(ps *PipelineState) error {
	state, _ := gr.store.LoadState(gr.goal.ID)
	state.PipelineState = &statestore.PipelineState{
		ResumePoint:      ps.ResumePoint,
		ResumePrimitive:  ps.ResumePrimitive,
		WaitReason:       ps.WaitReason,
		TimeoutAt:        ps.TimeoutAt,
		PendingActionIDs: ps.PendingActionIDs,
	}
	if err := gr.store.SaveSnapshot(gr.goal.ID, state); err != nil {
		return fmt.Errorf("goalrunner: save snapshot: %w", err)
	}
	return nil
}

// waitForWakeup 等待外部唤醒事件。
// 订阅对应事件类型，阻塞直到事件到达或超时。
func (gr *GoalRunner) waitForWakeup(result *PipelineResult) events.Event {
	eventType := wakeupEventForReason(result.WaitReason)
	ch := make(chan events.Event, 1)

	subID := gr.bus.SubscribeForGoal(gr.goal.ID, eventType, func(evt events.Event) error {
		ch <- evt
		return nil
	})
	defer gr.bus.Unsubscribe(subID)

	// 超时处理：Wait 不是永久阻塞。超时→返回 TimeoutEvent
	timeout := 5 * time.Minute
	if result.PipelineState != nil && result.PipelineState.TimeoutAt != "" {
		if t, err := time.Parse(time.RFC3339, result.PipelineState.TimeoutAt); err == nil {
			timeout = time.Until(t)
		}
	}

	select {
	case evt := <-ch:
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
func (gr *GoalRunner) publishGoalCompleted() {
	gr.bus.Publish(events.Event{
		Type:   events.TypeGoalCompleted,
		GoalID: gr.goal.ID,
		Source: "goalrunner",
	})
}

// publishGoalFailed 发布 GoalCompleted 事件（失败终态）。
func (gr *GoalRunner) publishGoalFailed(reason string) {
	gr.bus.Publish(events.Event{
		Type:   events.TypeGoalFailed,
		GoalID: gr.goal.ID,
		Source: "goalrunner",
		Payload: map[string]interface{}{
			"reason": reason,
		},
	})
}

// wakeupEventForReason 返回 Wait 原因对应的唤醒事件类型。
func wakeupEventForReason(reason string) string {
	switch reason {
	case "approval":
		return events.TypeUserApprovedAction
	case "dependency":
		return events.TypeActionCompleted
	case "resource":
		return events.TypeResourceAvailable
	default:
		return events.TypeGoalResumed
	}
}
