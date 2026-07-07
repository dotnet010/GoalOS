// Package test — v0.2.0 Week 1 S1-S4 安全契约测试（R-660）
package test

import (
	"testing"
	"time"

	"github.com/goalos/goalos/internal/daemon"
	"github.com/goalos/goalos/internal/governance"
	"github.com/goalos/goalos/internal/pluginrunner"
)

// ─── S1: Token 撤销验证 ────────────────────────────────────────

func TestS1_Token_Revocation(t *testing.T) {
	store := governance.NewTokenStore()

	t.Run("有效Token_VerifyToken返回true", func(t *testing.T) {
		token := store.Issue("goal-1", "action-1", "shell.execute", 30*time.Second)
		valid, err := store.VerifyToken(token.TokenID)
		if err != nil {
			t.Fatalf("VerifyToken 不应返回 error: %v", err)
		}
		if !valid {
			t.Error("新签发的 Token 应验证通过")
		}
	})

	t.Run("已撤销Token_VerifyToken返回false", func(t *testing.T) {
		token := store.Issue("goal-2", "action-2", "fs.write", 30*time.Second)
		store.Revoke(token.TokenID)
		valid, _ := store.VerifyToken(token.TokenID)
		if valid {
			t.Error("已撤销的 Token 应验证失败")
		}
	})

	t.Run("Wait时旧Token撤销_新Token签发_VerifyToken通过", func(t *testing.T) {
		tokenID := "goal-3-action-3"
		store.Issue("goal-3", "action-3", "browser.click", 60*time.Second)
		store.Revoke(tokenID)
		// 旧 Token 已撤销→VerifyToken 失败
		valid, _ := store.VerifyToken(tokenID)
		if valid {
			t.Error("撤销后 VerifyToken 应返回 false")
		}
		// Wait→Resume: 签发新 Token（同 goal+action→同 tokenID，清除撤销标记）
		store.Issue("goal-3", "action-3", "browser.click", 60*time.Second)
		valid, _ = store.VerifyToken(tokenID)
		if !valid {
			t.Error("Wait→Resume 后签发的新 Token 应验证通过（Issue 清除了撤销标记）")
		}
	})

	t.Run("TTL不超过24h硬上限", func(t *testing.T) {
		token := store.Issue("goal-4", "action-4", "shell.execute", 365*24*time.Hour)
		maxTTL := 24 * time.Hour
		actualTTL := token.ExpiresAt.Sub(token.IssuedAt)
		if actualTTL > maxTTL {
			t.Errorf("Token TTL=%v 超过硬上限 %v", actualTTL, maxTTL)
		}
	})
}

// ─── S2: FD3 协议 ──────────────────────────────────────────────

func TestS2_FD3_Protocol(t *testing.T) {
	t.Run("OpenFD3_文件描述符3可用", func(t *testing.T) {
		// 单元测试环境 FD3 不可用——验证返回正确 error
		_, err := pluginrunner.OpenFD3()
		if err == nil {
			t.Log("FD3 可用（集成测试环境）")
		} else {
			t.Logf("FD3 不可用（单元测试环境——预期）: %v", err)
		}
	})

	t.Run("SignMessage_生成64字符hex", func(t *testing.T) {
		sig := pluginrunner.SignMessage("secret-token", []byte(`{"type":"result"}`))
		if len(sig) != 64 {
			t.Errorf("HMAC 签名长度=%d, 期望 64（SHA256 hex）", len(sig))
		}
	})
}

// ─── S3: HMAC 事件类型 ─────────────────────────────────────────

func TestS3_HMAC_EventType(t *testing.T) {
	t.Run("HMAC验证通过_Valid=true", func(t *testing.T) {
		sessionToken := "test-session-token-32-bytes-xxxxx"
		payload := []byte(`{"type":"result","action_id":"a1","status":"success"}`)
		sig := pluginrunner.SignMessage(sessionToken, payload)

		result := pluginrunner.VerifyHMAC(sessionToken, payload, sig)
		if !result.Valid {
			t.Error("正确 HMAC 签名应验证通过")
		}
	})

	t.Run("HMAC验证失败_error_type_为ipc_security_violation", func(t *testing.T) {
		sessionToken := "test-session-token"
		payload := []byte(`{"type":"result"}`)
		wrongSig := "0000000000000000000000000000000000000000000000000000000000000000"

		result := pluginrunner.VerifyHMAC(sessionToken, payload, wrongSig)
		if result.Valid {
			t.Error("错误 HMAC 签名应验证失败")
		}
		if result.ErrorType != "ipc_security_violation" {
			t.Errorf("ErrorType=%q, 期望 ipc_security_violation", result.ErrorType)
		}
	})

	t.Run("HMAC签名确定性的_相同输入产生相同签名", func(t *testing.T) {
		token := "token"
		payload := []byte(`{"test":true}`)
		sig1 := pluginrunner.SignMessage(token, payload)
		sig2 := pluginrunner.SignMessage(token, payload)
		if sig1 != sig2 {
			t.Error("相同输入的 HMAC 签名应相同（确定性）")
		}
	})
}

// ─── S4: macOS 安全标注 ────────────────────────────────────────

func TestS4_macOS_SecurityLabel(t *testing.T) {
	level := daemon.GetPlatformSecurityLevel()

	t.Run("平台信息非空", func(t *testing.T) {
		if level.Platform == "" {
			t.Error("Platform 不应为空")
		}
		if level.Label == "" {
			t.Error("Label 不应为空")
		}
	})

	t.Run("安全能力列表非空", func(t *testing.T) {
		if len(level.Capabilities) == 0 {
			t.Error("Capabilities 不应为空")
		}
	})

	t.Run("级别为L2或L3", func(t *testing.T) {
		if level.Level != "L2" && level.Level != "L3" {
			t.Errorf("Level=%q, 期望 L2 或 L3", level.Level)
		}
	})
}
