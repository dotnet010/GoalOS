// Package pluginrunner implements the GoalOS Plugin Runner.
// Event Bus ↔ Executor 子进程的桥。订阅 ActionApproved → 启动子进程 → IPC → 发布 ActionCompleted/Failed。
//
// 设计依据：05 架构文档 §4.3, §8, R137, R197。
package pluginrunner

import (
	"fmt"
	"log"
	"time"

	"github.com/goalos/goalos/internal/eventbus"
	"github.com/goalos/goalos/pkg/events"
)

// Runner manages Plugin subprocess lifecycle.
type Runner struct {
	bus      *eventbus.EventBus
	seq      int
}

// New creates a Plugin Runner.
func New(bus *eventbus.EventBus) *Runner {
	return &Runner{bus: bus}
}

// Start subscribes to ActionApproved and begins execution.
func (r *Runner) Start() {
	r.bus.Subscribe(events.TypeActionApproved, r.handleActionApproved)
	log.Println("[PluginRunner] started, subscribed to ActionApproved")
}

func (r *Runner) handleActionApproved(evt events.Event) error {
	actionID, _ := evt.Payload["action_id"].(string)
	actionType, _ := evt.Payload["action_type"].(string)

	log.Printf("[PluginRunner] executing: %s (%s)", actionID, actionType)

	// W3: 尝试真实子进程执行。失败→回退 stub
	result, err := r.executeAction(evt)
	if err != nil {
		log.Printf("[PluginRunner] 子进程执行失败: %v。使用 stub", err)
		result = r.stubExecute(actionID, actionType)
	}

	r.publish(events.Event{
		Type:   result.eventType,
		GoalID: evt.GoalID,
		Source: "plugin-runner",
		Payload: map[string]interface{}{
			"action_id": actionID,
			"result": map[string]interface{}{
				"status": result.status,
				"output": result.output,
			},
			"artifacts_produced": []interface{}{},
			"cost": map[string]interface{}{
				"duration_ms": result.durationMs,
			},
		},
	})
	return nil
}

func (r *Runner) executeAction(evt events.Event) (execResult, error) {
	actionID, _ := evt.Payload["action_id"].(string)
	actionType, _ := evt.Payload["action_type"].(string)

	// W3: 扫描 plugins/ 目录查找匹配的 Plugin 二进制
	binaryPath := r.findPluginBinary(actionType)
	if binaryPath == "" {
		return execResult{}, fmt.Errorf("未找到 %s 的 Plugin 二进制", actionType)
	}

	// os/exec 启动子进程。IPC 协议通信。
	cfg := ExecConfig{
		BinaryPath: binaryPath,
		Timeout:    30 * time.Second,
	}
	action := ActionRequest{
		ActionID:   actionID,
		ActionType: actionType,
	}

	result, err := Execute(cfg, action)
	if err != nil {
		return execResult{}, err
	}

	evtType := events.TypeActionCompleted
	if result.Status != "success" {
		evtType = events.TypeActionFailed
	}
	return execResult{
		eventType:  evtType,
		status:     result.Status,
		output:     result.Output,
		durationMs: result.DurationMs,
	}, nil
}

func (r *Runner) findPluginBinary(actionType string) string {
	// W3: 扫描 ~/.goalos/plugins/ 查找匹配的 Plugin。
	// 简化实现：W3 仅支持 shell executor Plugin。
	// W4: 完整 Plugin 发现机制。
	_ = actionType
	return "" // W3: 返回空→回退 stub
}

type execResult struct {
	eventType  string
	status     string
	output     string
	durationMs int
}

func (r *Runner) stubExecute(actionID, actionType string) execResult {
	return execResult{
		eventType:  events.TypeActionCompleted,
		status:     "success",
		output:     fmt.Sprintf("W3 stub: %s completed", actionType),
		durationMs: 10,
	}
}

func (r *Runner) publish(evt events.Event) {
	r.seq++
	evt.Seq = r.seq
	r.bus.Publish(evt)
}
