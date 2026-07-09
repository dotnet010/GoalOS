// Package scheduler — v0.2.0 Week 2: GoalRunner 两阶段 select（R-702）
// 控制指令（Pause/Stop）优先于唤醒事件。
package scheduler

import (
	"sync"
)

// GoalControl 是 GoalRunner 接收的控制指令。
type GoalControl string

const (
	ControlPause GoalControl = "pause"
	ControlStop  GoalControl = "stop"
	ControlResume GoalControl = "resume"
)

// GoalState 是 Goal 的运行时状态。
type GoalState struct {
	GoalID      string
	State       string // Draft/Running/Paused/Failed/Completed
	ControlChan chan GoalControl
	WakeupChan  chan WakeupEvent
	mu          sync.Mutex
}

// WakeupEvent 是唤醒 GoalRunner 的事件。
type WakeupEvent struct {
	Type   string // "approval" | "dependency_met" | "resource_available"
	ActionID string
}

// NewGoalState 创建新的 Goal 运行时状态。
func NewGoalState(goalID string) *GoalState {
	return &GoalState{
		GoalID:      goalID,
		State:       "Draft",
		ControlChan: make(chan GoalControl, 10),
		WakeupChan:  make(chan WakeupEvent, 10),
	}
}

// Run 是 GoalRunner 的主循环——两阶段 select（R-702）。
// 第一阶段：select { controlChan, wakeupChan }——控制指令优先。
// 第二阶段：根据状态机转换执行相应操作。
func (gs *GoalState) Run(stopCh <-chan struct{}) {
	for {
		select {
		case <-stopCh:
			return
		case ctrl := <-gs.ControlChan:
			gs.handleControl(ctrl)
		default:
			select {
			case <-stopCh:
				return
			case ctrl := <-gs.ControlChan:
				// R-702: 控制指令优先
				gs.handleControl(ctrl)
			case evt := <-gs.WakeupChan:
				gs.handleWakeup(evt)
			}
		}
	}
}

// handleControl 处理控制指令。
func (gs *GoalState) handleControl(ctrl GoalControl) {
	gs.mu.Lock()
	defer gs.mu.Unlock()

	switch ctrl {
	case ControlPause:
		if gs.State == "Running" {
			gs.State = "Paused"
		}
	case ControlStop:
		gs.State = "Failed"
	case ControlResume:
		if gs.State == "Paused" {
			gs.State = "Running"
		}
	}
}

// handleWakeup 处理唤醒事件。
func (gs *GoalState) handleWakeup(evt WakeupEvent) {
	gs.mu.Lock()
	defer gs.mu.Unlock()

	if gs.State == "Paused" {
		return // 暂停状态下不处理唤醒
	}
	// H11: 根据唤醒类型更新状态
	switch evt.Type {
	case "approval", "dependency_met", "resource_available":
		if gs.State == "Running" {
			// 唤醒有效——GoalRunner 外部处理订阅和恢复
		}
	}
}

// IsTerminal 判断 Goal 是否处于终态（I3）。
func (gs *GoalState) IsTerminal() bool {
	gs.mu.Lock()
	defer gs.mu.Unlock()
	return gs.State == "Completed" || gs.State == "Failed"
}

// GetState 返回 Goal 当前状态（线程安全）。v0.2.0 audit fix.
func (gs *GoalState) GetState() string {
	gs.mu.Lock()
	defer gs.mu.Unlock()
	return gs.State
}

// SetState 设置 Goal 状态（线程安全）。v0.2.0 audit fix.
func (gs *GoalState) SetState(s string) {
	gs.mu.Lock()
	defer gs.mu.Unlock()
	gs.State = s
}
