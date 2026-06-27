// Package scheduler — RecoveryPipeline + BudgetTracker v0.1.0。
// 熔断后的自动恢复管线。决策树：exec_crash→RETRY→SWITCH_TOOL→ESCALATE。
// BudgetTracker: 追踪 Token/费用/重试次数/重复失败模式。
//
// 设计依据：05 架构文档 §3.12-3.13、R299、R334。

package scheduler

import (
	"fmt"
	"log"
	"sync"

	"github.com/goalos/goalos/internal/eventbus"
	"github.com/goalos/goalos/pkg/events"
)

// RecoveryPipeline 是熔断后的自动恢复管线（v0.1.0）。
// 在 ESCALATE 到用户之前，自动尝试多种技术方案。
type RecoveryPipeline struct {
	mu             sync.Mutex
	retryCount     map[string]int // actionID → 重试次数
	autoFixCount   map[string]int // actionID → 自修正次数
	switchToolUsed map[string]bool // actionID → 是否已尝试 SWITCH_TOOL
}

// NewRecoveryPipeline 创建恢复管线。
func NewRecoveryPipeline() *RecoveryPipeline {
	return &RecoveryPipeline{
		retryCount:     make(map[string]int),
		autoFixCount:   make(map[string]int),
		switchToolUsed: make(map[string]bool),
	}
}

// Decide 分析 ActionFailed 事件，选择恢复路径（v0.1.0 决策树）。
func (rp *RecoveryPipeline) Decide(actionID string, errorType string, budget *BudgetTracker, goalID string) RecoveryPath {
	rp.mu.Lock()
	defer rp.mu.Unlock()

	switch errorType {
	case "seccomp_violation", "policy_denied":
		// 安全事件——直接 ESCALATE。不重试。
		return RecoveryEscalate("security_violation: " + errorType)

	case "exec_crash", "exec_timeout":
		rp.retryCount[actionID]++
		if rp.retryCount[actionID] < 3 {
			backoff := rp.retryCount[actionID] // 1s, 2s, 3s
			return RecoveryRetry(actionID, rp.retryCount[actionID], backoff)
		}
		// 重试 3 次后→SWITCH_TOOL
		if !rp.switchToolUsed[actionID] {
			rp.switchToolUsed[actionID] = true
			return RecoverySwitchTool(actionID)
		}
		return RecoveryEscalate("retry_exhausted")

	case "accept_criteria_not_met":
		rp.autoFixCount[actionID]++
		if rp.autoFixCount[actionID] < 3 {
			return RecoveryAutoFix(actionID, rp.autoFixCount[actionID])
		}
		return RecoveryEscalate("auto_fix_exhausted")

	default:
		// 未知错误——直接 ESCALATE
		return RecoveryEscalate("unknown_error: " + errorType)
	}
}

// RecoveryPath 是恢复决策结果。
type RecoveryPath struct {
	Action     string `json:"action"`      // "RETRY"|"SWITCH_TOOL"|"AUTO_FIX"|"ESCALATE"
	ActionID   string `json:"action_id"`
	Attempt    int    `json:"attempt"`
	BackoffSec int    `json:"backoff_sec"`
	Reason     string `json:"reason"`
	Escalate   bool   `json:"escalate"` // true=需要人工介入
}

func RecoveryRetry(actionID string, attempt int, backoff int) RecoveryPath {
	return RecoveryPath{Action: "RETRY", ActionID: actionID, Attempt: attempt, BackoffSec: backoff}
}
func RecoverySwitchTool(actionID string) RecoveryPath {
	return RecoveryPath{Action: "SWITCH_TOOL", ActionID: actionID, Reason: "retry_exhausted"}
}
func RecoveryAutoFix(actionID string, attempt int) RecoveryPath {
	return RecoveryPath{Action: "AUTO_FIX", ActionID: actionID, Attempt: attempt}
}
func RecoveryEscalate(reason string) RecoveryPath {
	return RecoveryPath{Action: "ESCALATE", Reason: reason, Escalate: true}
}

// ErrorContext 是重试时的错误上下文摘要（v0.1.0 ≤200 token）。
type ErrorContext struct {
	ErrorType     string `json:"error_type"`     // "exec_crash"|"exec_timeout"|...
	Message       string `json:"message"`        // 简短描述。≤100 字符
	RetryCount    int    `json:"retry_count"`    // 当前重试次数
	AutoFixCount  int    `json:"auto_fix_count"` // 自修正次数
	PreviousResult string `json:"previous_result,omitempty"` // 上次执行结果。≤50 字符
}

// BudgetTracker 追踪 LLM Token 消耗和熔断（v0.1.0）。
// 仅追踪 LLM Token 和 API 费用——非 LLM 成本由 Action.Result.cost 上报。
type BudgetTracker struct {
	mu              sync.Mutex
	goals           map[string]*GoalBudget
	totalTokens     int
	totalCostUSD    float64
	bus             *eventbus.EventBus // v0.1.0 R-372: TokenUsage 事件发布
}

// GoalBudget 是单个 Goal 的预算追踪。
type GoalBudget struct {
	GoalID       string
	TokenLimit   int     // 默认 1,000,000
	TokensUsed   int
	CostLimitUSD float64 // 默认 $50
	CostUsedUSD  float64
	Tripped      bool    // 熔断已触发
}

// NewBudgetTracker 创建 BudgetTracker。
func NewBudgetTracker() *BudgetTracker {
	return &BudgetTracker{
		goals: make(map[string]*GoalBudget),
	}
}

// InitGoal 初始化 Goal 预算。
func (bt *BudgetTracker) InitGoal(goalID string, tokenLimit int) {
	bt.mu.Lock()
	defer bt.mu.Unlock()
	if _, ok := bt.goals[goalID]; !ok {
		if tokenLimit == 0 {
			tokenLimit = 1_000_000
		}
		bt.goals[goalID] = &GoalBudget{GoalID: goalID, TokenLimit: tokenLimit, CostLimitUSD: 50}
	}
}

// SetEventBus 设置事件总线（v0.1.0 R-372: TokenUsage 事件发布）。
func (bt *BudgetTracker) SetEventBus(bus *eventbus.EventBus) { bt.bus = bus }

// RecordTokens 记录 Token 消耗。返回 true 表示触发熔断。
func (bt *BudgetTracker) RecordTokens(goalID string, tokens int) bool {
	bt.mu.Lock()
	defer bt.mu.Unlock()

	b, ok := bt.goals[goalID]
	if !ok {
		b = &GoalBudget{GoalID: goalID, TokenLimit: 1_000_000, CostLimitUSD: 50}
		bt.goals[goalID] = b
	}

	b.TokensUsed += tokens
	bt.totalTokens += tokens

	// v0.1.0 R-372: 发布 TokenUsage 事件
	if bt.bus != nil {
		bt.bus.Publish(events.Event{
			Type:   events.TypeTokenUsage,
			GoalID: goalID,
			Source: "budget-tracker",
			Payload: map[string]interface{}{
				"tokens_used":   float64(tokens),
				"total_tokens":  float64(b.TokensUsed),
				"token_limit":   float64(b.TokenLimit),
				"tripped":       b.Tripped,
			},
		})
	}

	if b.TokensUsed >= b.TokenLimit {
		b.Tripped = true
		log.Printf("[BudgetTracker] goal=%s budget tripped: %d/%d tokens", goalID, b.TokensUsed, b.TokenLimit)
		return true
	}
	if b.TokensUsed >= b.TokenLimit*80/100 {
		log.Printf("[BudgetTracker] goal=%s budget warning: %d/%d tokens (80%%)", goalID, b.TokensUsed, b.TokenLimit)
	}
	return false
}

// IsTripped 检查 Goal 是否已熔断。
func (bt *BudgetTracker) IsTripped(goalID string) bool {
	bt.mu.Lock()
	defer bt.mu.Unlock()
	b, ok := bt.goals[goalID]
	return ok && b.Tripped
}

// Usage 返回当前使用统计。
func (bt *BudgetTracker) Usage(goalID string) (used int, limit int) {
	bt.mu.Lock()
	defer bt.mu.Unlock()
	b, ok := bt.goals[goalID]
	if !ok {
		return 0, 1_000_000
	}
	return b.TokensUsed, b.TokenLimit
}

// FormatErrorContext 格式化 ErrorContext 为 LLM 友好摘要（≤200 token）。
func FormatErrorContext(ec *ErrorContext) string {
	return fmt.Sprintf("错误类型: %s。%s。重试 #%d。自修正 #%d。上次结果: %s",
		ec.ErrorType, ec.Message, ec.RetryCount, ec.AutoFixCount,
		truncate(ec.PreviousResult, 50))
}

func truncate(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen] + "..."
}
