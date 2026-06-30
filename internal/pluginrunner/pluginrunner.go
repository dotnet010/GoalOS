// Package pluginrunner implements the GoalOS Plugin Runner.
// Event Bus ↔ Executor 子进程的桥。订阅 ActionApproved → 启动子进程 → IPC → 发布 ActionCompleted/Failed。
//
// 设计依据：05 架构文档 §4.3, §8, R137, R197。
package pluginrunner

import (
	"fmt"
	"log"
	"os"
	"strings"
	"time"

	"github.com/goalos/goalos/internal/eventbus"
	"github.com/goalos/goalos/internal/governance"
	"github.com/goalos/goalos/pkg/events"
)

// TokenVerifier 是 Token 验证接口。R-660: 支持撤销检查。
type TokenVerifier interface {
	VerifyToken(tokenStr string) (*governance.TokenClaims, error)
}

// Runner manages Plugin subprocess lifecycle.
type Runner struct {
	bus           *eventbus.EventBus
	discovery     *PluginDiscovery
	secretKey     []byte
	tokenVerifier TokenVerifier // R-660: 支持撤销检查的 Token 验证器
	seq           int
}

// New creates a Plugin Runner with the given plugins directory and token secret.
// tokenVerifier 可选——如果为 nil，使用 governance.VerifyToken（无撤销检查）。
func New(bus *eventbus.EventBus, secretKey []byte, tokenVerifier TokenVerifier) *Runner {
	home, _ := osUserHomeDir()
	pluginsDir := home + "/.goalos/plugins"
	return &Runner{
		bus:           bus,
		discovery:     NewPluginDiscovery(pluginsDir),
		secretKey:     secretKey,
		tokenVerifier: tokenVerifier,
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

	// Token 验证：如果 payload 含 token→校验签名+过期+撤销状态（R-660）
	if tokenStr, _ := evt.Payload["token"].(string); tokenStr != "" {
		var claims *governance.TokenClaims
		var err error
		if r.tokenVerifier != nil {
			claims, err = r.tokenVerifier.VerifyToken(tokenStr) // R-660: 含撤销检查
		} else if len(r.secretKey) > 0 {
			claims, err = governance.VerifyToken(tokenStr, r.secretKey) // fallback: 无撤销检查
		}
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
				"error_type": errorTypeFrom(err),
			},
		})
		return nil
	}

		// v0.1.0: output 为空时用 errMsg 填充
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
	riskLevel, _ := evt.Payload["risk_level"].(string) // v0.1.1 H4: 风险等级
	if riskLevel == "" {
		if dp, ok := evt.Payload["decision_path"].(map[string]interface{}); ok {
			if rl, ok := dp["risk"].(string); ok { riskLevel = rl }
		}
	}
	var requiredCaps []string
	if caps, ok := evt.Payload["required_capabilities"].([]interface{}); ok {
		for _, c := range caps {
			if s, ok := c.(string); ok { requiredCaps = append(requiredCaps, s) }
		}
	}

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
		RiskLevel:  riskLevel, // v0.1.1 H4: 传递给 SeccompForRiskLevel
	}
	action := ActionRequest{
		ActionID:             actionID,
		ActionType:           actionType,
		Target:               target,
		Params:               params,
		RequiredCapabilities: requiredCaps, // v0.1.1 H3: 传入 InitMessage
	}

	result, err := Execute(cfg, action)
	// R-660: 子进程退出后发布 PluginProcessTerminated——Capability Engine 监听此事件撤销所有 Token
	exitCode := 0
	reason := "completed"
	if err != nil {
		exitCode = -1
		reason = errorTypeFrom(err)
	}
	r.publish(events.Event{
		Type:   events.TypePluginProcessTerminated,
		GoalID: evt.GoalID,
		Source: "plugin-runner",
		Payload: map[string]interface{}{
			"plugin_name": plugin.Manifest.Name,
			"exit_code":   exitCode,
			"reason":      reason,
		},
	})
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

// errorTypeFrom 从 error 消息推断正确的 error_type。
// R-660+R-703: HMAC/IPC 安全违规必须返回 "ipc_security_violation"——非 "execution_error"。
func errorTypeFrom(err error) string {
	if err == nil {
		return "execution_error"
	}
	msg := err.Error()
	switch {
	case strings.Contains(msg, "ipc_security_violation") || strings.Contains(strings.ToLower(msg), "hmac"):
		return "ipc_security_violation"
	case strings.Contains(msg, "seccomp"):
		return "seccomp_violation"
	case strings.Contains(msg, "timeout"):
		return "timeout"
	case strings.Contains(msg, "crash") || strings.Contains(msg, "signal") || strings.Contains(msg, "killed"):
		return "crash"
	default:
		return "execution_error"
	}
}
