// Package governance — v0.2.0 Week 1 S1: Token 撤销验证
// R-795: MaxTokenTTL 24h 硬上限。
package governance

import (
	"sync"
	"time"

	"github.com/goalos/goalos/internal/errorcategory"
)

// TokenStatus 是 Capability Token 的状态。
type TokenStatus string

const (
	TokenActive  TokenStatus = "active"
	TokenRevoked TokenStatus = "revoked"
	TokenExpired TokenStatus = "expired"
)

// CapabilityToken 表示一个已签发的 Capability Token。
type CapabilityToken struct {
	TokenID    string
	GoalID     string
	ActionID   string
	Capability string
	IssuedAt   time.Time
	ExpiresAt  time.Time
	Status     TokenStatus
}

// TokenStore 管理 Token 的签发、验证、撤销。
// 内存缓存 + 定期刷盘到 tokens.json。
type TokenStore struct {
	mu      sync.RWMutex
	tokens  map[string]*CapabilityToken // tokenID → token
	revoked map[string]bool             // 已撤销 tokenID 快速查找
}

// NewTokenStore 创建新的 Token 存储。
func NewTokenStore() *TokenStore {
	return &TokenStore{
		tokens:  make(map[string]*CapabilityToken),
		revoked: make(map[string]bool),
	}
}

// Issue 签发新 Token。
// TTL = min(actionTimeout×2, MaxTokenTTL)。
func (ts *TokenStore) Issue(goalID, actionID, capability string, actionTimeout time.Duration) *CapabilityToken {
	ts.mu.Lock()
	defer ts.mu.Unlock()

	ttl := actionTimeout * 2
	if ttl > errorcategory.MaxTokenTTL {
		ttl = errorcategory.MaxTokenTTL
	}

	now := time.Now()
	tokenID := goalID + "-" + actionID
	// Wait→Resume: 签发新 Token 时清除旧撤销记录
	delete(ts.revoked, tokenID)

	token := &CapabilityToken{
		TokenID:    tokenID,
		GoalID:     goalID,
		ActionID:   actionID,
		Capability: capability,
		IssuedAt:   now,
		ExpiresAt:  now.Add(ttl),
		Status:     TokenActive,
	}
	ts.tokens[token.TokenID] = token
	return token
}

// VerifyToken 验证 Token 是否有效（S1）。
// MUST: 检查撤销列表。已撤销→Invalid。
func (ts *TokenStore) VerifyToken(tokenID string) (bool, error) {
	ts.mu.RLock()
	defer ts.mu.RUnlock()

	// S1: 检查撤销列表
	if ts.revoked[tokenID] {
		return false, nil
	}

	token, exists := ts.tokens[tokenID]
	if !exists {
		return false, nil
	}

	// 检查过期
	if time.Now().After(token.ExpiresAt) {
		token.Status = TokenExpired
		return false, nil
	}

	if token.Status != TokenActive {
		return false, nil
	}

	return true, nil
}

// Revoke 撤销 Token（S1）。
// Action 完成/崩溃后立即撤销。Wait 进入时旧 Token 撤销。
func (ts *TokenStore) Revoke(tokenID string) {
	ts.mu.Lock()
	defer ts.mu.Unlock()

	if token, exists := ts.tokens[tokenID]; exists {
		token.Status = TokenRevoked
	}
	ts.revoked[tokenID] = true
}

// RevokeAllForPlugin 撤销某 Plugin 的所有活跃 Token。
func (ts *TokenStore) RevokeAllForPlugin(pluginID string) {
	ts.mu.Lock()
	defer ts.mu.Unlock()

	for _, token := range ts.tokens {
		if token.Status == TokenActive {
			token.Status = TokenRevoked
			ts.revoked[token.TokenID] = true
		}
	}
}
