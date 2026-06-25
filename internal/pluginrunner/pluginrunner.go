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
	if tokenStr, _ := evt.Payload["token"].(string); tokenStr != "" && len(r.secretKey) > 0 {
		claims, err := governance.VerifyToken(tokenStr, r.secretKey)
		if err != nil {
			log.Printf("[PluginRunner] token verification failed: %v", err)
			r.publish(events.Event{
				Type:    events.TypeActionFailed,
				GoalID:  evt.GoalID,
				Source:  "plugin-runner",
				Payload: map[string]interface{}{
					"action_id":  actionID,
					"error":      fmt.Sprintf("token: %v", err),
					"error_type": "token_invalid",
				},
			})
			return nil
		}
		// Token scope 检查：Token 授权的 capability 是否覆盖本次 action_type
		if !tokenCoversAction(claims.Capabilities, actionType) {
			log.Printf("[PluginRunner] token scope: %v does not cover %s", claims.Capabilities, actionType)
			r.publish(events.Event{
				Type:    events.TypeActionFailed,
				GoalID:  evt.GoalID,
				Source:  "plugin-runner",
				Payload: map[string]interface{}{
					"action_id":  actionID,
					"error":      fmt.Sprintf("token scope: %v does not cover %s", claims.Capabilities, actionType),
					"error_type": "token_scope_denied",
				},
			})
			return nil
		}
	}

	// 尝试真实子进程执行。失败→回退 stub（MVP）。
	result, err := r.executeAction(evt)
	if err != nil {
		log.Printf("[PluginRunner] execution error: %v", err)
		r.publish(events.Event{
			Type:    events.TypeActionFailed,
			GoalID:  evt.GoalID,
			Source:  "plugin-runner",
			Payload: map[string]interface{}{
				"action_id": actionID,
				"result":    map[string]interface{}{"status": "failure", "output": fmt.Sprintf("no plugin for: %s", actionType)},
				"error":     fmt.Sprintf("execution failed: %v", err),
				"error_type": "execution_error",
			},
		})
		return nil
	}

		// v1.1.0: output 为空时用 errMsg 填充
	displayOutput := result.output
	if displayOutput == "" && result.errMsg != "" {
		displayOutput = result.errMsg
	}
	log.Printf("[PluginRunner] result: type=%s status=%s output_len=%d", result.eventType, result.status, len(displayOutput))

	r.publish(events.Event{
		Type:   result.eventType,
		GoalID: evt.GoalID,
		Source: "plugin-runner",
		Payload: map[string]interface{}{
			"action_id": actionID,
			"result": map[string]interface{}{
				"status": result.status,
				"output": displayOutput,
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
			errMsg:     result.Error,
		durationMs: result.DurationMs,
	}, nil
}

type execResult struct {
	eventType  string
	status     string
	output     string
	errMsg     string
	durationMs int
}

// tokenCoversAction 检查 Token 授权的 capability 列表是否覆盖 actionType。
func tokenCoversAction(capabilities []string, actionType string) bool {
	for _, c := range capabilities {
		if c == actionType {
			return true
		}
	}
	return false
}

func (r *Runner) publish(evt events.Event) {
	r.seq++
	evt.Seq = r.seq
	r.bus.Publish(evt)
}

// ─── OS Helper ───

var osUserHomeDir = os.UserHomeDir
