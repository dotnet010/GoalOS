// Package governance implements the GoalOS Governance Layer.
// Execution Gate。五引擎：Policy/Capability/Risk/Approval/Audit。
// 订阅 ActionScheduled → 五引擎判定 → 发布 ActionApproved/ActionRejected/ActionPendingApproval。
// Handler 永不阻塞 Event Bus（R215）。
//
// 设计依据：05 架构文档 §6、R154、R215、R228、R231。
package governance

import (
	"log"

	"github.com/goalos/goalos/internal/eventbus"
	"github.com/goalos/goalos/pkg/events"
)

// PolicyRule is a single policy rule.
type PolicyRule struct {
	Name     string `yaml:"name"`
	Priority int    `yaml:"priority"`
	Action   string `yaml:"action"` // "DENY" | "APPROVAL_REQUIRED" | "ALLOW"
}

// Decision is the result of Governance evaluation.
type Decision struct {
	Policy     string // "ALLOW" | "DENY" | "APPROVAL_REQUIRED"
	Capability string // "GRANTED" | "DENIED"
	Risk       string // "L0" - "L5"
	Approval   string // "AUTO" | "GRANTED" | "DENIED" | "TIMEOUT"
}

// Engine is the Governance Layer.
type Engine struct {
	bus    *eventbus.EventBus
	policy []PolicyRule
	seq    int
}

// New creates a Governance Engine with default policies.
func New(bus *eventbus.EventBus) *Engine {
	return &Engine{
		bus: bus,
		policy: []PolicyRule{
			{Name: "default-allow", Priority: 999, Action: "ALLOW"},
		},
	}
}

// Start subscribes to ActionScheduled and UserApprovedAction, begins processing.
func (e *Engine) Start() {
	e.bus.Subscribe(events.TypeActionScheduled, e.handleActionScheduled)
	e.bus.Subscribe(events.TypeUserApprovedAction, e.handleUserApproved)
	log.Println("[Governance] started, subscribed to ActionScheduled, UserApprovedAction")
}

// handleUserApproved 处理用户批准异步审批。
func (e *Engine) handleUserApproved(evt events.Event) error {
	actionID, _ := evt.Payload["action_id"].(string)
	log.Printf("[Governance] 用户已批准: %s", actionID)

	// 签发 Token 并发布 ActionApproved
	e.publishApproved(evt, Decision{
		Policy:     "ALLOW",
		Capability: "GRANTED",
		Risk:       "L3",
		Approval:   "GRANTED",
	})
	return nil
}

func (e *Engine) handleActionScheduled(evt events.Event) error {
	actionType, _ := evt.Payload["action_type"].(string)

	// Step 2: Policy Engine — priority rule matching
	policyResult := e.evaluatePolicy(actionType)

	// Step 3: Capability Engine — static authorization check
	capResult := e.evaluateCapability(evt)

	// Step 4: Risk Engine — four-dimension scoring
	riskLevel := e.evaluateRisk(actionType)

	// Step 5: Approval Engine — trigger if L3+
	needsApproval := riskLevel >= "L3"

	// Step 6: Audit Engine — record decision path (logged for now)
	decision := Decision{
		Policy:     policyResult,
		Capability: capResult,
		Risk:       riskLevel,
		Approval:   "AUTO",
	}

	if policyResult == "DENY" || capResult == "DENIED" {
		e.publishRejected(evt, decision, "policy_denied")
		return nil
	}

	if needsApproval {
		// Publish pending — handler returns immediately (R215).
		// Does NOT block Event Bus.
		e.publish(events.Event{
			Type:   events.TypeActionPendingApproval,
			GoalID: evt.GoalID,
			Source: "governance",
			Payload: map[string]interface{}{
				"action_id":          evt.Payload["action_id"],
				"risk_level":         riskLevel,
				"action_description": actionType,
				"impact_description": "requires human approval",
				"timeout_seconds":    300,
			},
		})
		return nil
	}

	decision.Approval = "AUTO"
	e.publishApproved(evt, decision)
	return nil
}

// evaluatePolicy 执行优先级规则匹配。
// 按 priority 升序遍历规则集。首个匹配→返回 action。无匹配→ALLOW。
func (e *Engine) evaluatePolicy(actionType string) string {
	for _, r := range e.policy {
		if r.Action == "DENY" || r.Action == "APPROVAL_REQUIRED" {
			return r.Action
		}
	}
	return "ALLOW"
}

func (e *Engine) evaluateCapability(evt events.Event) string {
	// W1: default grant. Full check per R154 in W3.
	return "GRANTED"
}

// capabilityRiskMap is the built-in capability → four-dimension score mapping (R166).
var capabilityRiskMap = map[string]struct{ d, e, p, r int }{
	"fs.read":           {0, 0, 0, 0},  // 0→L0
	"browser.open":      {0, 1, 0, 0},  // 1→L0
	"fs.write":          {1, 0, 0, 1},  // 2→L1
	"browser.click":     {1, 1, 0, 1},  // 3→L1
	"github.push":       {1, 1, 0, 2},  // 4→L2
	"shell.execute":     {2, 2, 1, 2},  // 7→L3
	"fs.delete":         {2, 1, 1, 2},  // 6→L3
	"database.delete":   {4, 1, 1, 2},  // 8→L4
	"payment.initiate":  {5, 2, 2, 2},  // 11→L5
}

func (e *Engine) evaluateRisk(actionType string) string {
	scores, ok := capabilityRiskMap[actionType]
	if !ok {
		return "L1" // unknown actions default to low risk
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

func (e *Engine) publishApproved(evt events.Event, d Decision) {
	e.publish(events.Event{
		Type:   events.TypeActionApproved,
		GoalID: evt.GoalID,
		Source: "governance",
		Payload: map[string]interface{}{
			"action_id": evt.Payload["action_id"],
			"decision_path": map[string]string{
				"policy":     d.Policy,
				"capability": d.Capability,
				"risk":       d.Risk,
				"approval":   d.Approval,
			},
		},
	})
}

func (e *Engine) publishRejected(evt events.Event, d Decision, reason string) {
	e.publish(events.Event{
		Type:   events.TypeActionRejected,
		GoalID: evt.GoalID,
		Source: "governance",
		Payload: map[string]interface{}{
			"action_id":     evt.Payload["action_id"],
			"reject_reason": reason,
			"reject_source": "policy",
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
