// Package pluginrunner implements the GoalOS Plugin Runner.
// Event Bus ↔ Executor 子进程的桥。订阅 ActionApproved → 启动子进程 → IPC → 发布 ActionCompleted/Failed。
//
// 设计依据：05 架构文档 §4.3, §8, R137, R197。
package pluginrunner

import (
	"fmt"
	"log"
	"os"
	"time"

	"github.com/goalos/goalos/internal/eventbus"
	"github.com/goalos/goalos/internal/governance"
	"github.com/goalos/goalos/pkg/events"
)

// Runner manages Plugin subprocess lifecycle.
type Runner struct {
	bus       *eventbus.EventBus
	discovery *PluginDiscovery
	secretKey []byte
	seq       int
}

// New creates a Plugin Runner with the given plugins directory and token secret.
func New(bus *eventbus.EventBus, secretKey []byte) *Runner {
	home, _ := osUserHomeDir()
	pluginsDir := home + "/.goalos/plugins"
	return &Runner{
		bus:       bus,
		discovery: NewPluginDiscovery(pluginsDir),
		secretKey: secretKey,
	}
}

// Start subscribes to ActionApproved, discovers and loads Plugins.
func (r *Runner) Start() {
	r.bus.Subscribe(events.TypeActionApproved, r.handleActionApproved)

	// 扫描 plugins/ 目录，发现已安装的 Plugin
	if err := r.discovery.Refresh(); err != nil {
		log.Printf("[PluginRunner] discovery refresh: %v", err)
	}
	plugins := r.discovery.List()
	log.Printf("[PluginRunner] started, discovered %d plugins", len(plugins))
	for _, p := range plugins {
		log.Printf("[PluginRunner]   %s/%s (v%s) — %v", p.Manifest.Type, p.Manifest.Name, p.Manifest.Version, p.Manifest.DeclaredCapabilities)
	}
}

// DiscoveredPlugins returns the list of discovered plugins (for capability registration).
func (r *Runner) DiscoveredPlugins() []DiscoveredPlugin {
	return r.discovery.List()
}

func (r *Runner) handleActionApproved(evt events.Event) error {
	actionID, _ := evt.Payload["action_id"].(string)
	actionType, _ := evt.Payload["action_type"].(string)

	log.Printf("[PluginRunner] executing: %s (%s)", actionID, actionType)

	// Token 验证：如果 payload 含 token_id→校验签名和过期
	if tokenID, _ := evt.Payload["token_id"].(string); tokenID != "" && len(r.secretKey) > 0 {
		if _, err := governance.VerifyToken(tokenID, r.secretKey); err != nil {
			log.Printf("[PluginRunner] token verification failed: %v", err)
			r.publish(events.Event{
				Type:   events.TypeActionFailed,
				GoalID: evt.GoalID,
				Source: "plugin-runner",
				Payload: map[string]interface{}{
					"action_id":  actionID,
					"error":      fmt.Sprintf("token verification failed: %v", err),
					"error_type": "token_invalid",
				},
			})
			return nil
		}
	}

	// 尝试真实子进程执行。失败→回退 stub（MVP）。
	result, err := r.executeAction(evt)
	if err != nil {
		log.Printf("[PluginRunner] subprocess execution failed: %v — falling back to stub", err)
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
	target, _ := evt.Payload["target"].(string)
	params, _ := evt.Payload["params"].(map[string]interface{})

	// 查找匹配的 Plugin
	plugin := r.discovery.Find(actionType)
	if plugin == nil {
		return execResult{}, fmt.Errorf("no plugin found for action type: %s", actionType)
	}

	home, _ := osUserHomeDir()
	cfg := ExecConfig{
		BinaryPath: plugin.BinaryPath,
		WorkDir:    home + "/Goals/" + evt.GoalID,
		TmpDir:     "/tmp/goalos/" + actionID,
		Timeout:    30 * time.Second,
	}
	action := ActionRequest{
		ActionID:   actionID,
		ActionType: actionType,
		Target:     target,
		Params:     params,
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

type execResult struct {
	eventType  string
	status     string
	output     string
	durationMs int
}

func (r *Runner) stubExecute(actionID, actionType string) execResult {
	return execResult{
		eventType:  events.TypeActionFailed,
		status:     "failure",
		output:     fmt.Sprintf("no plugin binary for: %s", actionType),
		durationMs: 0,
	}
}

func (r *Runner) publish(evt events.Event) {
	r.seq++
	evt.Seq = r.seq
	r.bus.Publish(evt)
}

// ─── OS Helper ───

var osUserHomeDir = os.UserHomeDir
