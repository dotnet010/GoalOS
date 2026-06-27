// ipc_security_test.go — HMAC Zero Trust IPC 安全契约测试（会议 #63）。
// TC-SEC-003~006: HMAC 签名/验证/缺失/重放防护。

package pluginrunner

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"testing"
)

func TestHMAC_ValidSignature(t *testing.T) {
	token := generateSessionToken()
	if len(token) != 64 {
		t.Fatalf("session_token must be 64 hex chars, got %d", len(token))
	}

	// 模拟子进程签名
	msg := map[string]interface{}{
		"type":      "result",
		"action_id": "test-001",
		"status":    "success",
	}
	payload, _ := json.Marshal(msg)
	msg["hmac"] = computeHMAC(payload, token)

	// 模拟 PluginRunner 验证
	finalJSON, _ := json.Marshal(msg)
	var received map[string]interface{}
	json.Unmarshal(finalJSON, &received)

	if err := verifyHMAC(finalJSON, received, token); err != nil {
		t.Fatalf("valid HMAC should pass: %v", err)
	}
}

func TestHMAC_InvalidSignature(t *testing.T) {
	token := generateSessionToken()
	wrongToken := generateSessionToken()

	msg := map[string]interface{}{
		"type":      "result",
		"action_id": "test-002",
		"status":    "success",
	}
	payload, _ := json.Marshal(msg)
	msg["hmac"] = computeHMAC(payload, wrongToken) // 用错误 token 签名

	finalJSON, _ := json.Marshal(msg)
	var received map[string]interface{}
	json.Unmarshal(finalJSON, &received)

	if err := verifyHMAC(finalJSON, received, token); err == nil {
		t.Fatal("HMAC with wrong token should be rejected")
	}
}

func TestHMAC_MissingHMAC(t *testing.T) {
	token := generateSessionToken()

	msg := map[string]interface{}{
		"type":      "result",
		"action_id": "test-003",
		"status":    "success",
	}
	// 故意不加 hmac 字段
	finalJSON, _ := json.Marshal(msg)
	var received map[string]interface{}
	json.Unmarshal(finalJSON, &received)

	if err := verifyHMAC(finalJSON, received, token); err == nil {
		t.Fatal("message without hmac should be rejected")
	}
}

func TestHMAC_TamperedMessage(t *testing.T) {
	token := generateSessionToken()

	// 签名时 status=success
	msg := map[string]interface{}{
		"type":      "result",
		"action_id": "test-004",
		"status":    "success",
	}
	payload, _ := json.Marshal(msg)
	msg["hmac"] = computeHMAC(payload, token)

	// 篡改 status=failure
	msg["status"] = "failure"
	delete(msg, "hmac")             // 移除旧 HMAC
	msg["hmac"] = computeHMAC(payload, token) // 使用原始 payload 的 HMAC（已过期）

	finalJSON, _ := json.Marshal(msg)
	var received map[string]interface{}
	json.Unmarshal(finalJSON, &received)

	if err := verifyHMAC(finalJSON, received, token); err == nil {
		t.Fatal("tampered message should be rejected")
	}
}

func TestHMAC_SessionTokenUnique(t *testing.T) {
	tokens := make(map[string]bool)
	for i := 0; i < 100; i++ {
		tok := generateSessionToken()
		if tokens[tok] {
			t.Fatal("session_token collision detected")
		}
		tokens[tok] = true
	}
}

func TestHMAC_Deterministic(t *testing.T) {
	// 相同 payload + 相同 token → 相同 HMAC
	token := "test-token-32-bytes-xxxxxxxxxxxxxx"
	payload := []byte(`{"action_id":"test","status":"success"}`)

	h1 := computeHMAC(payload, token)
	h2 := computeHMAC(payload, token)

	if h1 != h2 {
		t.Fatal("HMAC must be deterministic")
	}
}

func TestHMAC_DifferentPayloads(t *testing.T) {
	token := "test-token-32-bytes-xxxxxxxxxxxxxx"

	h1 := computeHMAC([]byte(`{"status":"success"}`), token)
	h2 := computeHMAC([]byte(`{"status":"failure"}`), token)

	if h1 == h2 {
		t.Fatal("different payloads must produce different HMACs")
	}
}

// 确保 HMAC 实现与 crypto/hmac 标准库一致
func TestHMAC_StandardLibraryCompatible(t *testing.T) {
	token := "secret"
	payload := []byte("hello")

	mac := hmac.New(sha256.New, []byte(token))
	mac.Write(payload)
	expected := hex.EncodeToString(mac.Sum(nil))

	actual := computeHMAC(payload, token)

	if actual != expected {
		t.Fatalf("HMAC mismatch: expected=%s actual=%s", expected, actual)
	}
}
