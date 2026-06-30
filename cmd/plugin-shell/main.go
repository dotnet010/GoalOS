// plugin-shell — GoalOS Capability Plugin: Shell Executor（v0.1.0 会议 #63 HMAC+seccomp）。
// stdin/stdout JSON 行协议 + HMAC Zero Trust IPC。
// 在 init 阶段自加载 seccomp（Linus 方案：子进程 post-fork 加载）。
package main

import (
	"bufio"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"time"

	"github.com/goalos/goalos/pkg/seccomp"
)

type ipcMessage struct {
	Type           string                 `json:"type"`
	SessionToken   string                 `json:"session_token,omitempty"`
	SeccompProfile *seccomp.Profile       `json:"seccomp_profile,omitempty"`
	Config         map[string]interface{} `json:"config,omitempty"`
	Capabilities   []string               `json:"capabilities,omitempty"`
	Workspace      string                 `json:"workspace,omitempty"`
	Tmp            string                 `json:"tmp,omitempty"`
	ActionID       string                 `json:"action_id,omitempty"`
	ActionType     string                 `json:"action_type,omitempty"`
	Target         string                 `json:"target,omitempty"`
	Params         map[string]interface{} `json:"params,omitempty"`
	TimeoutMs      int                    `json:"timeout_ms,omitempty"`
	Reason         string                 `json:"reason,omitempty"`
}

type resultMessage struct {
	Type     string `json:"type"`
	ActionID string `json:"action_id"`
	Status   string `json:"status"`
	Output   string `json:"output,omitempty"`
	Error    string `json:"error,omitempty"`
	CostMs   int    `json:"cost_ms"`
	// R-660: Hmac 字段已移除——v0.2.0 两行协议中 HMAC 作为独立行输出，不嵌入 JSON
}

type errorMessage struct {
	Type    string `json:"type"`
	Code    string `json:"code"`
	Message string `json:"message"`
	// R-660: Hmac 字段已移除——v0.2.0 两行协议中 HMAC 作为独立行输出
}

var sessionToken string

func main() {
	scanner := bufio.NewScanner(os.Stdin)
	scanner.Buffer(make([]byte, 65536), 65536)

	var initialized bool

	for scanner.Scan() {
		line := scanner.Bytes()
		var msg ipcMessage
		if err := json.Unmarshal(line, &msg); err != nil {
			writeError("invalid_message", fmt.Sprintf("cannot parse message: %v", err))
			continue
		}

		switch msg.Type {
		case "init":
			if msg.Workspace == "" {
				writeError("init_failed", "missing workspace path")
				os.Exit(1)
			}
			// 存储 session_token 用于后续 HMAC 签名
			sessionToken = msg.SessionToken

			// 自加载 seccomp（会议 #63 Linus 方案：子进程 post-fork 加载）
			if msg.SeccompProfile != nil {
				if err := seccomp.Apply(msg.SeccompProfile); err != nil {
					fmt.Fprintf(os.Stderr, "[plugin-shell] seccomp apply failed: %v\n", err)
				}
			}
			initialized = true

		case "execute":
			if !initialized {
				writeError("not_initialized", "received execute before init")
				continue
			}
			handleExecute(msg)

		case "shutdown":
			os.Exit(0)

		default:
			writeError("invalid_message", fmt.Sprintf("unknown message type: %s", msg.Type))
		}
	}
}

func handleExecute(msg ipcMessage) {
	start := time.Now()
	switch msg.ActionType {
	case "shell.execute", "shell.run":
		runShellCommand(msg)
	default:
		writeResult(msg.ActionID, "failure", "", fmt.Sprintf("unsupported action type: %s", msg.ActionType), int(time.Since(start).Milliseconds()))
	}
}

func runShellCommand(msg ipcMessage) {
	start := time.Now()
	cmdStr := msg.Target
	if cmdStr == "" {
		if p, ok := msg.Params["command"]; ok {
			cmdStr = fmt.Sprint(p)
		}
	}
	if cmdStr == "" {
		writeResult(msg.ActionID, "failure", "", "no command specified", int(time.Since(start).Milliseconds()))
		return
	}

	// R-660: shell 命令执行——seccomp 在 init 阶段已加载，限制可用 syscall。
	// sh -c 是 shell plugin 的核心功能。安全边界由 seccomp+文件系统隔离提供。
	cmd := exec.Command("sh", "-c", cmdStr)
	cmd.Dir = msg.Workspace
	cmd.Env = append(os.Environ(),
		"GOALOS_WORKSPACE="+msg.Workspace,
		"GOALOS_TMP="+msg.Tmp,
	)
	output, err := cmd.CombinedOutput()
	elapsed := int(time.Since(start).Milliseconds())

	if err != nil {
		writeResult(msg.ActionID, "failure", string(output),
			fmt.Sprintf("command failed (exit: %v): %s", err, strings.TrimSpace(string(output))), elapsed)
		return
	}
	writeResult(msg.ActionID, "success", string(output), "", elapsed)
}

func writeResult(actionID, status, output, errStr string, costMs int) {
	msg := resultMessage{Type: "result", ActionID: actionID, Status: status, Output: output, Error: errStr, CostMs: costMs}
	signAndWrite(msg)
}

func writeError(code, message string) {
	msg := errorMessage{Type: "error", Code: code, Message: message}
	signAndWrite(msg)
}

// signAndWrite 计算 HMAC 并写入 stdout（会议 #63 Zero Trust IPC）。
func signAndWrite(v interface{}) {
	data, _ := json.Marshal(v)
	// R-660: v0.2.0 两行协议——第一行 HMAC-SHA256 hex，第二行 JSON payload。
	// HMAC 不嵌入 JSON——作为独立行输出。
	if sessionToken != "" {
		sig := computeHMAC(data, sessionToken)
		fmt.Println(sig)
	}
	fmt.Println(string(data))
}

// computeHMAC 计算 HMAC-SHA256，返回 hex 编码字符串。
func computeHMAC(payload []byte, token string) string {
	mac := hmac.New(sha256.New, []byte(token))
	mac.Write(payload)
	return hex.EncodeToString(mac.Sum(nil))
}
