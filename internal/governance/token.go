// Package governance — Capability Token 签发与验证。
// JWT-like 格式：base64(header).base64(payload).base64(hmac-sha256-signature)
// daemon 持有唯一密钥；Token 通过 ActionApproved Payload 分发给 PluginRunner。
// 子进程可将 Token 用于回连 daemon API（为将来设计）。
//
// 设计依据：06 安全模型 §Token、R154、R226。
package governance

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"os"
	"time"
)

// TokenClaims 是 Capability Token 的 Payload。
type TokenClaims struct {
	GoalID       string   `json:"goal_id"`
	ActionID     string   `json:"action_id"`
	Capabilities []string `json:"capabilities"`
	IssuedAt     int64    `json:"iat"`
	ExpiresAt    int64    `json:"exp"`
}

// tokenHeader 是 Token 的固定 Header。
type tokenHeader struct {
	Alg string `json:"alg"` // "HS256"
	Typ string `json:"typ"` // "GOALOS-CAP"
}

// IssueToken 签发 Capability Token。
// claims: Token 声明；secret: daemon 密钥（32 字节）。
// 返回 base64(header).base64(payload).base64(signature) 格式的 Token 字符串。
func IssueToken(claims TokenClaims, secret []byte) (string, error) {
	header := tokenHeader{Alg: "HS256", Typ: "GOALOS-CAP"}
	headerJSON, err := json.Marshal(header)
	if err != nil {
		return "", fmt.Errorf("token: marshal header: %w", err)
	}
	payloadJSON, err := json.Marshal(claims)
	if err != nil {
		return "", fmt.Errorf("token: marshal payload: %w", err)
	}

	headerB64 := base64URLEncode(headerJSON)
	payloadB64 := base64URLEncode(payloadJSON)
	signingInput := headerB64 + "." + payloadB64

	sig := sign(signingInput, secret)
	sigB64 := base64URLEncode(sig)

	return signingInput + "." + sigB64, nil
}

// VerifyToken 验证 Capability Token 的签名和过期时间。
// 返回解析后的 TokenClaims。签名无效或已过期时返回 error。
func VerifyToken(tokenStr string, secret []byte) (*TokenClaims, error) {
	// 解析 token
	parts := splitToken(tokenStr)
	if len(parts) != 3 {
		return nil, fmt.Errorf("token: 格式无效：期望 3 段，实际 %d 段", len(parts))
	}

	headerB64, payloadB64, sigB64 := parts[0], parts[1], parts[2]
	signingInput := headerB64 + "." + payloadB64

	// 验证签名
	expectedSig := sign(signingInput, secret)
	actualSig, err := base64URLDecode(sigB64)
	if err != nil {
		return nil, fmt.Errorf("token: 签名解码失败: %w", err)
	}
	if !hmac.Equal(expectedSig, actualSig) {
		return nil, fmt.Errorf("token: 签名验证失败")
	}

	// 解析 payload
	payloadJSON, err := base64URLDecode(payloadB64)
	if err != nil {
		return nil, fmt.Errorf("token: payload 解码失败: %w", err)
	}
	var claims TokenClaims
	if err := json.Unmarshal(payloadJSON, &claims); err != nil {
		return nil, fmt.Errorf("token: payload 解析失败: %w", err)
	}

	// 验证过期
	if time.Now().Unix() > claims.ExpiresAt {
		return nil, fmt.Errorf("token: 已过期 (exp=%d)", claims.ExpiresAt)
	}

	return &claims, nil
}

// sign 计算 HMAC-SHA256 签名。
func sign(input string, secret []byte) []byte {
	mac := hmac.New(sha256.New, secret)
	mac.Write([]byte(input))
	return mac.Sum(nil)
}

// base64URLEncode 是 URL-safe Base64 编码（无填充）。
func base64URLEncode(data []byte) string {
	return base64.RawURLEncoding.EncodeToString(data)
}

// base64URLDecode 是 URL-safe Base64 解码。
func base64URLDecode(s string) ([]byte, error) {
	return base64.RawURLEncoding.DecodeString(s)
}

// splitToken 按 "." 分割 Token 字符串。
func splitToken(token string) []string {
	parts := make([]string, 0, 3)
	start := 0
	for i := 0; i < len(token); i++ {
		if token[i] == '.' {
			parts = append(parts, token[start:i])
			start = i + 1
		}
	}
	parts = append(parts, token[start:])
	return parts
}

// ─── Secret Key 管理 ──────────────────────────────────────────

// LoadOrGenerateSecret 从 keyPath 加载 daemon 密钥；不存在时生成新密钥并写入文件。
// 返回 32 字节 HMAC-SHA256 密钥。
func LoadOrGenerateSecret(keyPath string) ([]byte, error) {
	// 尝试读取已有密钥
	if data, err := os.ReadFile(keyPath); err == nil && len(data) == 32 {
		return data, nil
	}

	// 生成新密钥
	secret := make([]byte, 32)
	if _, err := rand.Read(secret); err != nil {
		return nil, fmt.Errorf("token: 生成密钥失败: %w", err)
	}

	// 写入文件
	if err := os.WriteFile(keyPath, secret, 0600); err != nil {
		return nil, fmt.Errorf("token: 写入密钥文件失败: %w", err)
	}

	return secret, nil
}
