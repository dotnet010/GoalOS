// Package pluginrunner — IPC 子进程执行器（v0.1.0 会议 #63 Linus 方案重写）。
// os/exec 启动独立子进程。stdin/stdout JSON 行协议。
// seccomp 由子进程在 init 阶段自加载（非父进程注入）。
// HMAC Zero Trust IPC：每条消息附带 HMAC-SHA256(payload, session_token)。
//
// 设计依据：08 沙箱与 IPC 规范 §2-§3、R137、R197、会议 #63。
package pluginrunner

import (
	"bufio"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"os"
	"os/exec"
	"syscall"
	"time"

	"github.com/goalos/goalos/pkg/seccomp"
)

// ExecConfig 子进程执行配置。
type ExecConfig struct {
	BinaryPath string
	Args       []string
	WorkDir    string
	TmpDir     string
	Timeout    time.Duration
	RiskLevel  string
}

// ExecResult 子进程执行结果。
type ExecResult struct {
	ActionID   string `json:"action_id"`
	Status     string `json:"status"`
	Output     string `json:"output"`
	Error      string `json:"error"`
	DurationMs int    `json:"cost_ms"`
}

// Execute 启动子进程（v0.1.0 会议 #63 重写）。
// seccomp profile 通过 InitMessage 传给子进程自加载。
// session_token 用于 HMAC Zero Trust IPC。
func Execute(cfg ExecConfig, action ActionRequest) (*ExecResult, error) {
	cmd := exec.Command(cfg.BinaryPath, cfg.Args...)
	if cfg.WorkDir != "" {
		cmd.Dir = cfg.WorkDir
	}
	cmd.Env = append(os.Environ(),
		"GOALOS_WORKSPACE="+cfg.WorkDir,
		"GOALOS_TMP="+cfg.TmpDir,
	)

	stdin, err := cmd.StdinPipe()
	if err != nil {
		return nil, fmt.Errorf("executor: stdin pipe: %w", err)
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return nil, fmt.Errorf("executor: stdout pipe: %w", err)
	}

	// 子进程安全加固（Linux: CLONE_NEWNET + Pdeathsig。macOS: Setpgid）
	// 不再在此处 ApplySeccomp——由子进程在 init 阶段自加载（会议 #63 Linus 方案）
	sanitizeChildProcess(cmd)

	if cfg.WorkDir != "" {
		if err := os.MkdirAll(cfg.WorkDir, 0755); err != nil {
			return nil, fmt.Errorf("executor: mkdir: %w", err)
		}
	}

	// 生成 session_token（64 字符随机 hex）
	sessionToken := generateSessionToken()

	// 选择 seccomp profile
	seccompProfile := seccomp.ForRiskLevel(cfg.RiskLevel)

	startTime := time.Now()
	if err := cmd.Start(); err != nil {
		return nil, fmt.Errorf("executor: start: %w", err)
	}

	// 发送 init（含 session_token + seccomp_profile + protocol_version）
	writeJSON(stdin, map[string]interface{}{
		"type":             "init",
		"protocol_version": "v2.0-two-line-hmac", // R-724: 内部协议版本。Plugin 必须支持此版本
		"session_token":    sessionToken,
		"seccomp_profile":  seccompProfile,
		"capabilities":     action.RequiredCapabilities,
		"workspace":        cfg.WorkDir,
		"tmp":              cfg.TmpDir,
	})

	resultCh := make(chan *ExecResult, 1)
	errCh := make(chan error, 1)
	go readStdout(stdout, resultCh, errCh, sessionToken)

	log.Printf("[executor] sending execute: action=%s type=%s target_len=%d", action.ActionID, action.ActionType, len(action.Target))
	writeJSON(stdin, map[string]interface{}{
		"type":        "execute",
		"action_id":   action.ActionID,
		"action_type": action.ActionType,
		"target":      action.Target,
		"params":      action.Params,
		"timeout_ms":  cfg.Timeout.Milliseconds(),
	})

	var result *ExecResult
	select {
	case r := <-resultCh:
		result = r
	case e := <-errCh:
		return nil, e
	case <-time.After(cfg.Timeout):
		cmd.Process.Signal(syscall.SIGTERM)
		select {
		case r := <-resultCh:
			result = r
		case <-time.After(5 * time.Second):
			cmd.Process.Kill()
			return nil, fmt.Errorf("executor: timeout (%v)", cfg.Timeout)
		}
	}

	writeJSON(stdin, map[string]interface{}{
		"type":   "shutdown",
		"reason": "completed",
	})
	stdin.Close()
	cmd.Wait()

	result.DurationMs = int(time.Since(startTime).Milliseconds())
	return result, nil
}

// readStdout 从 stdout 读取子进程返回的消息（v0.1.0 HMAC 验证）。
func readStdout(stdout io.Reader, resultCh chan<- *ExecResult, errCh chan<- error, sessionToken string) {
	scanner := bufio.NewScanner(stdout)
	scanner.Buffer(make([]byte, 65536), 65536)
	for scanner.Scan() {
		line := scanner.Bytes()

		// R-660: v0.2.0 两行 HMAC 协议——第一行 HMAC-SHA256 hex，第二行 JSON payload。
		// HMAC 不嵌入 JSON——作为独立行输出。
		var msg map[string]interface{}
		if sessionToken != "" && isHexLine(line) {
			// 第一行是 HMAC hex——读取第二行（JSON payload）
			hmacLine := string(line)
			if !scanner.Scan() {
				errCh <- fmt.Errorf("ipc_security_violation: expected JSON payload after HMAC line")
				return
			}
			jsonLine := scanner.Bytes()
			if err := json.Unmarshal(jsonLine, &msg); err != nil {
				errCh <- fmt.Errorf("ipc_security_violation: invalid JSON after HMAC line: %w", err)
				return
			}
			// 验证 HMAC（基于 JSON payload 原始 bytes + sessionToken）
			if err := verifyTwoLineHMAC(hmacLine, jsonLine, sessionToken); err != nil {
				log.Printf("[executor] HMAC verification failed: %v", err)
				errCh <- fmt.Errorf("ipc_security_violation: HMAC verification failed")
				return
			}
		} else {
			// 旧协议兼容：单行 JSON（HMAC 嵌入 JSON 或 无 HMAC）
			if err := json.Unmarshal(line, &msg); err != nil {
				continue
			}
			if sessionToken != "" {
				// 旧协议：HMAC 在 JSON 的 "hmac" 字段中
				if err := verifyHMAC(line, msg, sessionToken); err != nil {
					log.Printf("[executor] HMAC verification failed (legacy protocol): %v", err)
					errCh <- fmt.Errorf("ipc_security_violation: HMAC verification failed")
					return
				}
			}
		}

		msgType, _ := msg["type"].(string)
		switch msgType {
		case "result":
			r := &ExecResult{
				ActionID: str(msg["action_id"]),
				Status:   str(msg["status"]),
			}
			if v, ok := msg["output"].(string); ok {
				r.Output = v
			}
			if v, ok := msg["error"].(string); ok {
				r.Error = v
			}
			resultCh <- r
			return
		case "error":
			errCh <- fmt.Errorf("plugin error: %v", msg["message"])
			return
		}
	}
	if err := scanner.Err(); err != nil {
		errCh <- fmt.Errorf("stdout read: %w", err)
	}
}

// verifyHMAC 验证消息的 HMAC-SHA256 签名（会议 #63）。
func verifyHMAC(rawJSON []byte, msg map[string]interface{}, token string) error {
	receivedHMAC, _ := msg["hmac"].(string)
	if receivedHMAC == "" {
		return fmt.Errorf("missing hmac field")
	}
	// 重新解析原始 JSON 以保留字段顺序
	var raw map[string]interface{}
	json.Unmarshal(rawJSON, &raw)
	delete(raw, "hmac")
	payload, _ := json.Marshal(raw)
	expectedHMAC := computeHMAC(payload, token)
	if !hmac.Equal([]byte(expectedHMAC), []byte(receivedHMAC)) {
		return fmt.Errorf("hmac mismatch")
	}
	return nil
}

// computeHMAC 计算 HMAC-SHA256。
func computeHMAC(payload []byte, token string) string {
	mac := hmac.New(sha256.New, []byte(token))
	mac.Write(payload)
	return hex.EncodeToString(mac.Sum(nil))
}

// generateSessionToken 生成 64 字符随机 hex token。
func generateSessionToken() string {
	b := make([]byte, 32)
	rand.Read(b)
	return hex.EncodeToString(b)
}

func str(v interface{}) string {
	s, _ := v.(string)
	return s
}

// ActionRequest 发送给子进程的 Action 描述。
type ActionRequest struct {
	ActionID             string
	ActionType           string
	Target               string
	Params               map[string]interface{}
	RequiredCapabilities []string
}

func writeJSON(w io.Writer, v interface{}) error {
	data, err := json.Marshal(v)
	if err != nil {
		return err
	}
	data = append(data, '\n')
	_, err = w.Write(data)
	return err
}

// isHexLine 判断一行是否是 HMAC hex 签名行（64 字符 hex）。
func isHexLine(line []byte) bool {
	if len(line) != 64 {
		return false
	}
	for _, c := range line {
		if !((c >= '0' && c <= '9') || (c >= 'a' && c <= 'f') || (c >= 'A' && c <= 'F')) {
			return false
		}
	}
	return true
}

// verifyTwoLineHMAC 验证 v0.2.0 两行协议的 HMAC 签名。
// hmacHex: 第一行的 HMAC-SHA256 hex 字符串（64 字符）
// payload: 第二行的 JSON payload 原始 bytes
func verifyTwoLineHMAC(hmacHex string, payload []byte, token string) error {
	expectedHMAC, err := hex.DecodeString(hmacHex)
	if err != nil {
		return fmt.Errorf("hmac decode: %w", err)
	}
	mac := hmac.New(sha256.New, []byte(token))
	mac.Write(payload)
	actualHMAC := mac.Sum(nil)
	if !hmac.Equal(expectedHMAC, actualHMAC) {
		return fmt.Errorf("HMAC mismatch")
	}
	return nil
}
