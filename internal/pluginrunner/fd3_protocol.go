// Package pluginrunner — v0.2.0 Week 1 S2+S3: FD3 协议 + HMAC 事件类型
package pluginrunner

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"

	"github.com/goalos/goalos/internal/pluginrunner/ipc"
)

// FD3 是 Plugin 子进程的 IPC 控制通道文件描述符。
const FD3 = 3

// OpenFD3 从 daemon 端打开 FD3 控制通道（S2）。
// daemon 通过 cmd.ExtraFiles 将 FD3 传递给子进程。
func OpenFD3() (*os.File, error) {
	f := os.NewFile(FD3, "ipc")
	if f == nil {
		return nil, fmt.Errorf("FD3 not available")
	}
	return f, nil
}

// HMACResult 是 HMAC 验证结果。
type HMACResult struct {
	Valid     bool
	ErrorType string // "ipc_security_violation" | ""
}

// VerifyHMAC 验证子进程消息的 HMAC 签名（S3）。
// sessionToken: InitMessage 中下发的单次密钥。
// message: 子进程发送的原始 JSON payload（第二行）。
// signature: 子进程发送的 HMAC hex string（第一行）。
func VerifyHMAC(sessionToken string, message []byte, signature string) HMACResult {
	mac := hmac.New(sha256.New, []byte(sessionToken))
	mac.Write(message)
	expected := hex.EncodeToString(mac.Sum(nil))

	if !hmac.Equal([]byte(signature), []byte(expected)) {
		// S3: HMAC 失败→error_type="ipc_security_violation"，非 "execution_error"
		return HMACResult{
			Valid:     false,
			ErrorType: "ipc_security_violation",
		}
	}
	return HMACResult{Valid: true}
}

// SignMessage 对消息进行 HMAC 签名（Plugin SDK 端使用）。
func SignMessage(sessionToken string, payload []byte) string {
	mac := hmac.New(sha256.New, []byte(sessionToken))
	mac.Write(payload)
	return hex.EncodeToString(mac.Sum(nil))
}

// GenerateSessionToken 生成安全的 session_token（R-812）。
// 32 字节 crypto/rand，hex 编码→64 字符。
// R-503 + R-796: contract_test 验证 1000 次零碰撞。
func GenerateSessionToken() (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", fmt.Errorf("session_token: crypto/rand failed: %w", err)
	}
	return hex.EncodeToString(b), nil
}

// UnmarshalPayload — RawFrame.PayloadLine→Message（R-1447——传输层职责收窄后的协议层职责）。
// 契约：VerifyHMAC 通过后调用——反序列化=协议动作，不是字节搬运。
func UnmarshalPayload(rf *ipc.RawFrame) (*ipc.Message, error) {
	var msg ipc.Message
	if err := json.Unmarshal(rf.PayloadLine, &msg); err != nil {
		return nil, fmt.Errorf("protocol: JSON 反序列化失败: %w", err)
	}
	return &msg, nil
}
