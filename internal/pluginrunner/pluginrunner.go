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
	goruntime "runtime"

	goalosruntime "github.com/goalos/goalos/internal/runtime"
	"github.com/goalos/goalos/internal/sandbox"
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

	// v0.3.1 执行门（R-1640② 激活——会议 #255/#257）：
	contractVerifier *goalosruntime.ContractVerifier // 契约验证强制门（验签/时效/吊销/ProfileDigest/字段）
	resolver         *goalosruntime.Resolver         // 解析留痕（RuntimeSelected/Rejected 事件——5.5 数据源）
	policyRevision   string                          // 策略版本（digest 复核输入——缺省 builtin-v1）
}

// SetRuntimeGate 接线 Runtime 执行门（daemon 组合根注入——nil=门未激活=旧路径；
// 激活后：契约验证失败/解析拒绝=阻断执行（fail-closed）；Nonce 消费不激活——
// 凭据时序句①消费点=ExecutionSession 建立（Provider 路径收敛窗口落地——R-1640② 注记）。
func (r *Runner) SetRuntimeGate(cv *goalosruntime.ContractVerifier, res *goalosruntime.Resolver, policyRevision string) {
	r.contractVerifier = cv
	r.resolver = res
	r.policyRevision = policyRevision
}

// New creates a Plugin Runner with the given plugins directory and token secret.
// tokenVerifier 可选——如果为 nil，使用 governance.VerifyToken（无撤销检查）。
// 插件发现目录: GOALOS_PLUGINS_DIR 环境变量覆盖（测试隔离——沙箱模拟开关同先例
// R-929/R-960；未设置时默认 ~/.goalos/plugins）。
func New(bus *eventbus.EventBus, secretKey []byte, tokenVerifier TokenVerifier) *Runner {
	pluginsDir := os.Getenv("GOALOS_PLUGINS_DIR")
	if pluginsDir == "" {
		home, err := osUserHomeDir()
		if err != nil {
			home = "/tmp" // fallback: 容器环境
		}
		pluginsDir = home + "/.goalos/plugins"
	}
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

	// v0.2.0 W2: 真实子进程执行。失败→发布 ActionFailed（Decide 原语接管决策）。
	// R-817: 已删除 stub 回退路径。失败即诚实失败。
	result, err := r.executeAction(evt)
	if err != nil {
		log.Printf("[PluginRunner] execution error: %v", err)
		// R-828 final: flat payload
		payload := events.ActionCompletedPayload{
			ActionID: actionID,
			Status:   "failure",
			Output:   fmt.Sprintf("no plugin for: %s", actionType),
		}
		r.publish(events.Event{
			Type:    events.TypeActionFailed,
			GoalID:  evt.GoalID,
			Source:  "plugin-runner",
			Payload: events.PayloadToMap(payload),
			})
		return nil
	}

		// v0.1.0: output 为空时用 errMsg 填充
	displayOutput := result.output
	if displayOutput == "" && result.errMsg != "" {
		displayOutput = result.errMsg
	}
	log.Printf("[PluginRunner] result: type=%s status=%s output_len=%d", result.eventType, result.status, len(displayOutput))

	// R-828 final: flat typed payload——重构所有订阅者同步
	payload := events.ActionCompletedPayload{
		ActionID:   actionID,
		Status:     result.status,
		Output:     displayOutput,
		DurationMs: result.durationMs,
	}
	r.publish(events.Event{
		Type:    result.eventType,
		GoalID:  evt.GoalID,
		Source:  "plugin-runner",
		Payload: events.PayloadToMap(payload),
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

	// ─── v0.3.1 Runtime 执行门（R-1640② 激活）───
	if err := r.runtimeGate(evt, plugin); err != nil {
		return execResult{}, err
	}

	home, err := osUserHomeDir()
	if err != nil {
		return execResult{}, fmt.Errorf("pluginrunner: cannot determine home directory: %w", err)
	}
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

// runtimeGate Runtime 执行门（R-1640②——契约验证强制+解析留痕）：
// ①验证强制：Verify（验签/时效/吊销/v2 字段）→ProfileDigest 复核（永远独立重算——
// D-1 PM 裁决硬约束，不读签发侧缓存）→VerifyWithProfile 比对（不一致=默认拒绝）；
// ②解析留痕：Resolver.Resolve（WorkloadIdentity=插件二进制哈希——R-1561）；
// 拒绝（行 5/6 无候选/严禁降档）=阻断；行 3b 平台落差=事件留痕+放行
// （治理升级链落地前——D-2 裁决待定，PM 决策后收紧）。
func (r *Runner) runtimeGate(evt events.Event, plugin *DiscoveredPlugin) error {
	if r.contractVerifier == nil {
		return nil // 门未激活（无密钥环境）——旧路径
	}
	tokenStr, _ := evt.Payload["token"].(string)
	if tokenStr == "" {
		// 契约驱动执行（05 §X.6——无契约不执行）：门已激活而无 token=fail-closed
		return fmt.Errorf("runtime gate: 无契约 token——契约驱动执行（05 §X.6）")
	}
	vc, err := r.contractVerifier.Verify(tokenStr)
	if err != nil {
		return fmt.Errorf("runtime gate: 契约验证失败: %w", err)
	}
	claims := vc.Claims()
	rev := r.policyRevision
	if rev == "" {
		rev = "builtin-v1"
	}
	var platform sandbox.PlatformID
	switch goruntime.GOOS {
	case "darwin":
		platform = sandbox.PlatformDarwin
	case "windows":
		platform = sandbox.PlatformWindows
	default:
		platform = sandbox.PlatformLinux
	}
	freshDigest, _, derr := sandbox.BuiltinProfileDigest(claims.MinIsolation, claims.Capabilities, platform, rev)
	if derr != nil {
		return fmt.Errorf("runtime gate: profile 复核重算失败: %w", derr)
	}
	if _, err := r.contractVerifier.VerifyWithProfile(tokenStr, freshDigest); err != nil {
		return fmt.Errorf("runtime gate: ProfileDigest 复核不一致——默认拒绝+重新评估（05 §X.6.3）: %w", err)
	}
	if r.resolver == nil {
		return nil
	}
	minIso, perr := goalosruntime.ParseIsolationLevel(claims.MinIsolation)
	if perr != nil {
		return fmt.Errorf("runtime gate: MinIsolation 非法 %q: %w", claims.MinIsolation, perr)
	}
	var whash string
	if h, herr := governance.WorkloadHashOf(plugin.BinaryPath); herr == nil {
		whash = h
	}
	sel, serr := r.resolver.Resolve(goalosruntime.ResolveInput{
		RequiresRealEnforcement: claims.RequiresRealEnforcement,
		MinIsolation:            minIso,
		WorkloadHashHex:         whash,
	})
	if serr != nil {
		// 行 5/6：无满足候选=拒绝——严禁降级到更弱档（05 §X.6.4）
		return fmt.Errorf("runtime gate: 解析拒绝（严禁降档——05 §X.6.4 行 5/6）: %w", serr)
	}
	if sel.NeedsGovernanceEscalation {
		// 行 3b 平台落差：治理升级链落地前=留痕+放行（D-2 裁决待定——裁决后收紧为审批路由）
		log.Printf("[PluginRunner] runtime gate: 平台落差治理升级标记——%s（D-2 裁决待定，留痕放行）", sel.PlatformGap)
	}
	return nil
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
	// E17: nil error 不应返回 "execution_error"
	if err == nil {
		return ""
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
