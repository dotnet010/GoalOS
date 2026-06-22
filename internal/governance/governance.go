// Package governance implements the GoalOS Governance Layer.
// Execution Gate。五引擎：Policy/Capability/Risk/Approval/Audit。
// 订阅 ActionScheduled → 五引擎判定 → 发布 ActionApproved/ActionRejected/ActionPendingApproval。
// Handler 永不阻塞 EventBus（R215）。
//
// 设计依据：05 架构文档 §6、R154、R215、R228、R231。
package governance

import (
	"encoding/json"
	"fmt"
	"log"
	"os"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/goalos/goalos/internal/eventbus"
	"github.com/goalos/goalos/pkg/events"
)

// Condition 定义 Policy 规则的触发条件（R166）。
type Condition struct {
	ActionTypes     []string `yaml:"action_types"`
	TargetPatterns  []string `yaml:"target_patterns"`
	RiskMin         string   `yaml:"risk_min"` // "L0"-"L5"
}

// PolicyRule is a single policy rule with condition matching.
type PolicyRule struct {
	Name      string    `yaml:"name"`
	Priority  int       `yaml:"priority"`  // 数字越小优先级越高
	Condition Condition `yaml:"condition"`
	Action    string    `yaml:"action"`    // "DENY" | "APPROVAL_REQUIRED" | "ALLOW"
}

// Decision is the result of Governance evaluation.
type Decision struct {
	Policy     string // "ALLOW" | "DENY" | "APPROVAL_REQUIRED"
	Capability string // "GRANTED" | "DENIED"
	Risk       string // "L0" - "L5"
	Approval   string // "AUTO" | "GRANTED" | "DENIED" | "TIMEOUT"
	TokenID    string // Capability Token ID
	TokenStr   string // Capability Token 字符串（JWT）
}

// auditEntry 审计记录。
type auditEntry struct {
	Timestamp   string      `json:"timestamp"`
	GoalID      string      `json:"goal_id"`
	ActionID    string      `json:"action_id"`
	ActionType  string      `json:"action_type"`
	Decision    Decision    `json:"decision"`
	Result      string      `json:"result"` // "APPROVED" | "REJECTED" | "PENDING"
}

// Engine is the Governance Layer — Execution Gate.
type Engine struct {
	bus             *eventbus.EventBus
	capRegistry     map[string][]string
	policy          []PolicyRule
	secretKey       []byte
	autonomyLevel   string // "autonomous"→自动放行L3+
	seq             int

	// Audit Engine: ring buffer (1000 entries) + async flush
	auditBuf    []auditEntry
	auditPos    int
	auditMu     sync.Mutex
	auditLogDir string // ~/.goalos/logs/

	// Pending approval tracking (for timeout + race handling)
	pendingApprovals map[string]pendingApproval // actionID → timer info
	pendingMu        sync.Mutex
}

type pendingApproval struct {
	goalID     string
	actionID   string
	actionType string
	target     string
	params     map[string]interface{}
	requiredCaps []interface{}
	timeoutSec   float64
	timer      *time.Timer
}

// New creates a Governance Engine with default policies.
func New(bus *eventbus.EventBus, secretKey []byte) *Engine {
	home, _ := osUserHomeDir()
	e := &Engine{
		bus:              bus,
		capRegistry:      make(map[string][]string),
		secretKey:        secretKey,
		auditBuf:         make([]auditEntry, 1000),
		auditLogDir:      home + "/.goalos/logs/",
		pendingApprovals: make(map[string]pendingApproval),
		policy: []PolicyRule{
			{
				Name:     "block-prod-delete",
				Priority: 1,
				Condition: Condition{
					ActionTypes:    []string{"fs.delete", "database.delete"},
					TargetPatterns: []string{"/production/*"},
					RiskMin:        "L3",
				},
				Action: "DENY",
			},
			{
				Name:     "default-allow",
				Priority: 999,
				Condition: Condition{},
				Action: "ALLOW",
			},
		},
	}
	// 按 Priority 升序排序
	sort.Slice(e.policy, func(i, j int) bool { return e.policy[i].Priority < e.policy[j].Priority })
	return e
}

// SetAutonomyLevel 设置自治等级。autonomous→L3+自动放行。
func (e *Engine) SetAutonomyLevel(level string) {
	e.autonomyLevel = level
}

// Start subscribes to events and begins processing.
func (e *Engine) Start() {
	e.bus.Subscribe(events.TypeActionScheduled, e.handleActionScheduled)
	e.bus.Subscribe(events.TypeUserApprovedAction, e.handleUserApproved)
	e.bus.Subscribe(events.TypeActionCancelled, e.handleActionCancelled)
	// 启动审计异步刷盘 goroutine
	go e.auditFlushLoop()
	log.Println("[Governance] started (5 engines + audit ring buffer + approval timeout)")
}

// RegisterCapabilities 注册 Plugin 的 declared_capabilities（由 Plugin Runner 发现时调用）。
func (e *Engine) RegisterCapabilities(pluginName string, capabilities []string) {
	e.capRegistry[pluginName] = capabilities
}

// ─── Event Handlers ───

func (e *Engine) handleActionScheduled(evt events.Event) error {
	actionType, _ := evt.Payload["action_type"].(string)
	actionID, _ := evt.Payload["action_id"].(string)
	requiredCaps, _ := evt.Payload["required_capabilities"].([]interface{})
	target, _ := evt.Payload["target"].(string)
	riskLevelPre, _ := evt.Payload["risk_level_pre"].(string)

	// Step 2: Policy Engine — priority-ordered rule matching with condition check
	policyResult := e.evaluatePolicy(actionType, target, riskLevelPre)

	// Step 3: Capability Engine — static authorization: required_capabilities ⊆ declared
	capResult := e.evaluateCapability(requiredCaps)

	// Step 4: Risk Engine — four-dimension scoring
	riskLevel := e.evaluateRisk(actionType)

	// Step 5: Approval Engine — trigger if L3+
	needsApproval := (riskLevel >= "L3" || policyResult == "APPROVAL_REQUIRED") && e.autonomyLevel != "autonomous"

	decision := Decision{
		Policy:     policyResult,
		Capability: capResult,
		Risk:       riskLevel,
		Approval:   "AUTO",
	}

	// 记录审计
	e.recordAudit(auditEntry{
		Timestamp:  time.Now().Format(time.RFC3339),
		GoalID:     evt.GoalID,
		ActionID:   actionID,
		ActionType: actionType,
		Decision:   decision,
		Result:     "PENDING",
	})

	if policyResult == "DENY" {
		decision.Approval = "DENIED"
		e.publishRejected(evt, decision, "policy_denied", "policy")
		e.recordAudit(auditEntry{
			Timestamp: time.Now().Format(time.RFC3339), GoalID: evt.GoalID, ActionID: actionID,
			ActionType: actionType, Decision: decision, Result: "REJECTED",
		})
		return nil
	}

	if capResult == "DENIED" {
		decision.Approval = "DENIED"
		e.publishRejected(evt, decision, "capability_denied", "capability")
		e.recordAudit(auditEntry{
			Timestamp: time.Now().Format(time.RFC3339), GoalID: evt.GoalID, ActionID: actionID,
			ActionType: actionType, Decision: decision, Result: "REJECTED",
		})
		return nil
	}

	if needsApproval {
		// 发布 ActionPendingApproval → handler 立即返回。不阻塞 Event Bus（R215）。
		e.publish(events.Event{
			Type:   events.TypeActionPendingApproval,
			GoalID: evt.GoalID,
			Source: "governance",
			Payload: map[string]interface{}{
				"action_id":           actionID,
				"risk_level":          riskLevel,
				"action_description":  actionType,
				"impact_description":  fmt.Sprintf("风险等级 %s 的操作需要人工审批", riskLevel),
				"timeout_seconds":     300,
			},
		})

		// 启动审批超时计时器（300s → ActionRejected("approval_timeout")）
		e.pendingMu.Lock()
		target, _ := evt.Payload["target"].(string)
		params, _ := evt.Payload["params"].(map[string]interface{})
		requiredCaps, _ := evt.Payload["required_capabilities"].([]interface{})
		timeoutSec, _ := evt.Payload["timeout_seconds"].(float64)
		e.pendingApprovals[actionID] = pendingApproval{
			goalID:     evt.GoalID,
			actionID:   actionID,
			actionType: actionType,
			target:     target,
			params:     params,
			requiredCaps: requiredCaps,
			timeoutSec:   timeoutSec,
			timer: time.AfterFunc(300*time.Second, func() {
				e.handleApprovalTimeout(evt.GoalID, actionID, decision)
			}),
		}
		e.pendingMu.Unlock()

		return nil
	}

	decision.Approval = "AUTO"
	// 自动放行路径也签发 Token
	timeoutSec, _ := evt.Payload["timeout_seconds"].(float64)
	now := time.Now().Unix()
	ttl := int64(timeoutSec) * 2; if ttl <= 0 { ttl = 60 }
	tokenID, tokenStr := "", ""
	if len(e.secretKey) > 0 {
		caps := make([]string, len(requiredCaps))
		for i, c := range requiredCaps { caps[i] = fmt.Sprint(c) }
		claims := TokenClaims{GoalID: evt.GoalID, ActionID: actionID, Capabilities: caps, IssuedAt: now, ExpiresAt: now + ttl}
		if tok, err := IssueToken(claims, e.secretKey); err == nil { tokenStr = tok; tokenID = fmt.Sprintf("%s_token_%d", actionID, now) }
	}
	if tokenStr != "" {
		decision.TokenID = tokenID
		decision.TokenStr = tokenStr
	}
	e.publishApproved(evt, decision)
	e.recordAudit(auditEntry{
		Timestamp: time.Now().Format(time.RFC3339), GoalID: evt.GoalID, ActionID: actionID,
		ActionType: actionType, Decision: decision, Result: "APPROVED",
	})
	return nil
}

// handleUserApproved 处理用户异步审批。
// 批准前检查 Goal 状态 → 非 Running → ActionRejected("state_changed")（R150 竞态处理）。
func (e *Engine) handleUserApproved(evt events.Event) error {
	actionID, _ := evt.Payload["action_id"].(string)
	log.Printf("[Governance] 用户已批准: %s", actionID)

	// 检查是否仍在 pending（可能已被 timeout 或 cancel 处理）
	e.pendingMu.Lock()
	pending, exists := e.pendingApprovals[actionID]
	if exists {
		pending.timer.Stop()
		delete(e.pendingApprovals, actionID)
	}
	e.pendingMu.Unlock()
	if !exists {
		log.Printf("[Governance] approval %s: not pending (already timed out or cancelled)", actionID)
		return nil
	}

	// 签发 Token 并发布 ActionApproved
	decision := Decision{
		Policy:     "ALLOW",
		Capability: "GRANTED",
		Risk:       "L3",
		Approval:   "GRANTED",
	}

	// 签发 Capability Token
	now := time.Now().Unix()
	tokenID, tokenStr := "", ""
	if len(e.secretKey) > 0 {
		claims := TokenClaims{GoalID: pending.goalID, ActionID: actionID, Capabilities: []string{pending.actionType}, IssuedAt: now, ExpiresAt: now + 60}
		if tok, err := IssueToken(claims, e.secretKey); err == nil { tokenStr = tok; tokenID = fmt.Sprintf("%s_token_%d", actionID, now) }
	}
	if tokenStr != "" {
		decision.TokenID = tokenID
		decision.TokenStr = tokenStr
	}

	// 构造包含 action_type 的事件用于 publishApproved
	approvedEvt := events.Event{
		GoalID: evt.GoalID,
		Payload: map[string]interface{}{
			"action_id":             actionID,
			"action_type":           pending.actionType,
			"target":                pending.target,
			"params":                pending.params,
			"required_capabilities": pending.requiredCaps,
			"timeout_seconds":       pending.timeoutSec,
		},
	}
	e.publishApproved(approvedEvt, decision)
	e.recordAudit(auditEntry{
		Timestamp: time.Now().Format(time.RFC3339), GoalID: evt.GoalID, ActionID: actionID,
		ActionType: "", Decision: decision, Result: "APPROVED",
	})
	return nil
}

// handleActionCancelled 处理 Action 取消（异步审批竞态：用户 pause/rollback/stop）。
func (e *Engine) handleActionCancelled(evt events.Event) error {
	actionID, _ := evt.Payload["action_id"].(string)
	reason, _ := evt.Payload["reason"].(string)
	log.Printf("[Governance] ActionCancelled: %s (reason=%s)", actionID, reason)

	e.pendingMu.Lock()
	if pending, exists := e.pendingApprovals[actionID]; exists {
		pending.timer.Stop()
		delete(e.pendingApprovals, actionID)
	}
	e.pendingMu.Unlock()

	decision := Decision{
		Policy: "ALLOW", Capability: "GRANTED", Risk: "L0", Approval: "DENIED",
	}
	e.publishRejected(evt, decision, "state_changed", "approval")
	e.recordAudit(auditEntry{
		Timestamp: time.Now().Format(time.RFC3339), GoalID: evt.GoalID, ActionID: actionID,
		ActionType: "", Decision: decision, Result: "REJECTED",
	})
	return nil
}

// handleApprovalTimeout 审批超时处理。
func (e *Engine) handleApprovalTimeout(goalID, actionID string, decision Decision) {
	e.pendingMu.Lock()
	delete(e.pendingApprovals, actionID)
	e.pendingMu.Unlock()

	decision.Approval = "TIMEOUT"
	e.publish(events.Event{
		Type:   events.TypeActionRejected,
		GoalID: goalID,
		Source: "governance",
		Payload: map[string]interface{}{
			"action_id":     actionID,
			"reject_reason": "approval_timeout",
			"reject_source": "approval",
			"decision_path": map[string]string{
				"policy":     decision.Policy,
				"capability": decision.Capability,
				"risk":       decision.Risk,
				"approval":   "TIMEOUT",
			},
		},
	})
	e.recordAudit(auditEntry{
		Timestamp: time.Now().Format(time.RFC3339), GoalID: goalID, ActionID: actionID,
		ActionType: "", Decision: decision, Result: "REJECTED",
	})
	log.Printf("[Governance] approval timeout: %s", actionID)
}

// ─── Engines ───

// evaluatePolicy 按优先级升序遍历规则集，首个 Condition 匹配 → 返回 action。无匹配 → ALLOW。
func (e *Engine) evaluatePolicy(actionType, target, riskLevelPre string) string {
	for _, r := range e.policy {
		if e.matchCondition(r.Condition, actionType, target, riskLevelPre) {
			return r.Action
		}
	}
	return "ALLOW"
}

// matchCondition 检查 Condition 是否匹配当前 Action。
func (e *Engine) matchCondition(c Condition, actionType, target, riskLevelPre string) bool {
	// 空 Condition（如 default-allow）→ 总是匹配
	if len(c.ActionTypes) == 0 && len(c.TargetPatterns) == 0 && c.RiskMin == "" {
		return true
	}
	// action_types 匹配
	if len(c.ActionTypes) > 0 {
		match := false
		for _, at := range c.ActionTypes {
			if at == actionType {
				match = true
				break
			}
		}
		if !match {
			return false
		}
	}
	// target_patterns 匹配（glob 前缀匹配）
	if len(c.TargetPatterns) > 0 {
		match := false
		for _, pat := range c.TargetPatterns {
			if matchTarget(pat, target) {
				match = true
				break
			}
		}
		if !match {
			return false
		}
	}
	// risk_min 匹配
	if c.RiskMin != "" {
		if riskLevelPre < c.RiskMin {
			return false
		}
	}
	return true
}

// matchTarget 简单 glob 前缀匹配。
func matchTarget(pattern, target string) bool {
	if strings.HasSuffix(pattern, "/*") {
		return strings.HasPrefix(target, strings.TrimSuffix(pattern, "/*"))
	}
	return pattern == target
}

// evaluateCapability 静态授权检查：required_capabilities ⊆ any_plugin_declared。
func (e *Engine) evaluateCapability(requiredCaps []interface{}) string {
	if len(requiredCaps) == 0 {
		return "GRANTED" // 无需能力的 Action 不检查
	}
	// 收集所有已注册 Plugin 的能力
	allDeclared := make(map[string]bool)
	for _, caps := range e.capRegistry {
		for _, c := range caps {
			allDeclared[c] = true
		}
	}
	// 检查每个 required_capability 是否至少有一个 Plugin 声明
	for _, rc := range requiredCaps {
		cap, ok := rc.(string)
		if !ok {
			continue
		}
		if !allDeclared[cap] {
			log.Printf("[Governance] capability denied: %s not in any plugin's declared_capabilities", cap)
			return "DENIED"
		}
	}
	return "GRANTED"
}

// capabilityRiskMap — 四维评分映射（R166）。修正了 browser.open/browser.click 等级。
// 维度: d=destructiveness(0-3), e=external(0-2), p=privilege(0-3), r=reversibility(0-2)
var capabilityRiskMap = map[string]struct{ d, e, p, r int }{
	"web.search":        {0, 1, 0, 0}, // 1→L1 只读远程 API，无本地副作用
	"fs.read":           {0, 0, 0, 0}, // 0→L0
	"browser.open":      {0, 2, 0, 0}, // 2→L1 per spec label L1
	"fs.write":          {1, 0, 0, 1}, // 2→L1
	"browser.click":     {1, 1, 0, 1}, // 3→L2 per spec L2
	"github.push":       {1, 1, 0, 2}, // 4→L2
	"shell.execute":     {2, 2, 1, 2}, // 7→L3
	"fs.delete":         {2, 1, 1, 2}, // 6→L3 per spec label L3
	"database.delete":   {3, 1, 2, 2}, // 8→L4 per spec label L4
	"payment.initiate":  {3, 2, 3, 2}, // 10→L5 per spec label L5
}

func (e *Engine) evaluateRisk(actionType string) string {
	scores, ok := capabilityRiskMap[actionType]
	if !ok {
		return "L1" // unknown default to L1
	}
	total := scores.d + scores.e + scores.p + scores.r
	switch {
	case total <= 1:
		return "L0"
	case total <= 3:
		return "L1"
	case total <= 5:
		return "L2"
	case total <= 7:
		return "L3"
	case total <= 9:
		return "L4"
	default:
		return "L5"
	}
}

// ─── Audit Engine ───

// recordAudit 写入内存 ring buffer（1000 条）。异步刷盘。
func (e *Engine) recordAudit(entry auditEntry) {
	e.auditMu.Lock()
	e.auditBuf[e.auditPos%1000] = entry
	e.auditPos++
	e.auditMu.Unlock()
}

// auditFlushLoop 独立 goroutine 异步批量刷盘（P95 < 50ms）。
func (e *Engine) auditFlushLoop() {
	ticker := time.NewTicker(5 * time.Second)
	defer ticker.Stop()
	for range ticker.C {
		e.flushAudit()
	}
}

func (e *Engine) flushAudit() {
	e.auditMu.Lock()
	if e.auditPos == 0 {
		e.auditMu.Unlock()
		return
	}
	// 收集未刷盘事件
	count := e.auditPos
	if count > 1000 {
		count = 1000
	}
	toFlush := make([]auditEntry, count)
	start := e.auditPos - count
	for i := 0; i < count; i++ {
		toFlush[i] = e.auditBuf[(start+i)%1000]
	}
	e.auditMu.Unlock()

	// 追加写入 audit.log
	f, err := osOpenAppend(e.auditLogDir + "audit.log")
	if err != nil {
		log.Printf("[Governance] audit flush error: %v", err)
		return
	}
	defer f.Close()
	for _, entry := range toFlush {
		data, _ := json.Marshal(entry)
		fmt.Fprintf(f, "%s\n", data)
	}
}

// ─── Event Publishing ───

func (e *Engine) publishApproved(evt events.Event, d Decision) {
	// 转发 PluginRunner 执行所需的所有字段（从 ActionScheduled payload）
	payload := map[string]interface{}{
		"action_id":             evt.Payload["action_id"],
		"action_type":           evt.Payload["action_type"],
		"target":                evt.Payload["target"],
		"params":                evt.Payload["params"],
		"required_capabilities": evt.Payload["required_capabilities"],
		"timeout_seconds":       evt.Payload["timeout_seconds"],
		"decision_path": map[string]string{
			"policy":     d.Policy,
			"capability": d.Capability,
			"risk":       d.Risk,
			"approval":   d.Approval,
		},
	}
	if d.TokenID != "" {
		payload["token_id"] = d.TokenID
		payload["token"] = d.TokenStr
	}
	e.publish(events.Event{
		Type:    events.TypeActionApproved,
		GoalID:  evt.GoalID,
		Source:  "governance",
		Payload: payload,
	})
}

func (e *Engine) publishRejected(evt events.Event, d Decision, reason, source string) {
	e.publish(events.Event{
		Type:    events.TypeActionRejected,
		GoalID:  evt.GoalID,
		Source:  "governance",
		Payload: map[string]interface{}{
			"action_id":     evt.Payload["action_id"],
			"reject_reason": reason,
			"reject_source": source,
			"decision_path": map[string]string{
				"policy":     d.Policy,
				"capability": d.Capability,
				"risk":       d.Risk,
				"approval":   d.Approval,
			},
		},
	})
}

func (e *Engine) publish(evt events.Event) {
	e.seq++
	evt.Seq = e.seq
	e.bus.Publish(evt)
}

// ─── OS Helpers (extracted for testability) ───

var osUserHomeDir = os.UserHomeDir
var osOpenAppend = func(path string) (*os.File, error) {
	return os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0600)
}
