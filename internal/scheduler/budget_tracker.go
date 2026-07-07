// Package scheduler — v0.2.0 Week 3 B14+J5+E1: BudgetTracker扩展 + Token续期 + Evaluator
package scheduler

import (
	"sync"
	"time"
)

// ─── BudgetTracker Week 3 扩展方法 ────────────────────────────

// NewBudgetTrackerWithBudget 创建带初始预算的 BudgetTracker（Week 3 B14）。
// globalBudget 存储 NewBudgetTrackerWithBudget 传入的全局预算上限。
func NewBudgetTrackerWithBudget(budget int64) *BudgetTracker {
	bt := NewBudgetTracker()
	bt.totalTokens = 0
	bt.globalBudget = budget
	return bt
}

// RecordUsageSimple 简单记录 Token 消耗（Week 3 简化版——全局计数）。
func (bt *BudgetTracker) RecordUsageSimple(tokens int64) {
	bt.mu.Lock()
	defer bt.mu.Unlock()
	bt.totalTokens += int(tokens)
}

// Used 返回已消耗 Token 数。
func (bt *BudgetTracker) Used() int64 {
	bt.mu.Lock()
	defer bt.mu.Unlock()
	return int64(bt.totalTokens)
}

// IsExceededSimple 检查是否超过全局预算。
func (bt *BudgetTracker) IsExceededSimple(budget int64) bool {
	bt.mu.Lock()
	defer bt.mu.Unlock()
	return int64(bt.totalTokens) > budget
}

// AddBudgetSimple 追加全局预算。
func (bt *BudgetTracker) AddBudgetSimple(amount int64) {
	bt.mu.Lock()
	defer bt.mu.Unlock()
	// R-538: 追加 = 叠加。增加全局 Token 池以便 IsExceededSimple 正确判断。
	// 通过调整 totalTokens 的"透支"来反映预算增加。
	bt.totalTokens -= int(amount)
	if bt.totalTokens < 0 {
		bt.totalTokens = 0
	}
}

// UsageRatioSimple 返回全局预算使用率。
func (bt *BudgetTracker) UsageRatioSimple(budget int64) float64 {
	bt.mu.Lock()
	defer bt.mu.Unlock()
	if budget == 0 {
		return 1.0
	}
	return float64(bt.totalTokens) / float64(budget)
}

// ResetSimple 重置全局计数器。
func (bt *BudgetTracker) ResetSimple() {
	bt.mu.Lock()
	defer bt.mu.Unlock()
	bt.totalTokens = 0
}

// ─── J5: Token 续期 ────────────────────────────────────────────

// CapToken 是 Capability Token。
type CapToken struct {
	TokenID    string
	GoalID     string
	ActionID   string
	Capability string
	IssuedAt   time.Time
	ExpiresAt  time.Time
}

// NewCapToken 创建新 Token（J5：TTL = timeout×2）。
func NewCapToken(goalID, actionID, capability string, timeout time.Duration) *CapToken {
	now := time.Now()
	return &CapToken{
		TokenID:    goalID + "-" + actionID,
		GoalID:     goalID,
		ActionID:   actionID,
		Capability: capability,
		IssuedAt:   now,
		ExpiresAt:  now.Add(timeout * 2),
	}
}

// TokenRenewalStore 管理 Token 的签发、撤销、重签（J5 Wait→Resume 续期）。
type TokenRenewalStore struct {
	mu      sync.RWMutex
	tokens  map[string]*CapToken
	revoked map[string]bool
}

// NewTokenRenewalStore 创建新的 Token 续期存储。
func NewTokenRenewalStore() *TokenRenewalStore {
	return &TokenRenewalStore{
		tokens:  make(map[string]*CapToken),
		revoked: make(map[string]bool),
	}
}

// Issue 签发 Token。Wait→Resume 时重新签发（清除撤销标记）。
func (s *TokenRenewalStore) Issue(goalID, actionID, capability string, timeout time.Duration) *CapToken {
	s.mu.Lock()
	defer s.mu.Unlock()

	tokenID := goalID + "-" + actionID
	delete(s.revoked, tokenID) // 清除旧撤销标记

	token := NewCapToken(goalID, actionID, capability, timeout)
	s.tokens[tokenID] = token
	return token
}

// Revoke 撤销 Token（Wait 进入时）。
func (s *TokenRenewalStore) Revoke(tokenID string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.revoked[tokenID] = true
}

// IsRevoked 检查 Token 是否已撤销。
func (s *TokenRenewalStore) IsRevoked(tokenID string) bool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.revoked[tokenID]
}

// ─── E1: Evaluator ─────────────────────────────────────────────

// EvalResult 是评估结果。
type EvalResult string

const (
	EvalPASS EvalResult = "PASS"
	EvalFAIL EvalResult = "FAIL"
)

// BinaryEvaluator 二元评估器——比较实际输出与期望输出。
type BinaryEvaluator struct{}

// NewBinaryEvaluator 创建新的二元评估器。
func NewBinaryEvaluator() *BinaryEvaluator {
	return &BinaryEvaluator{}
}

// Evaluate 比较实际输出与期望输出。
func (e *BinaryEvaluator) Evaluate(actual, expected string) EvalResult {
	if actual == expected {
		return EvalPASS
	}
	return EvalFAIL
}
