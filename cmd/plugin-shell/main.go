// plugin-shell — GoalOS Capability Plugin: Shell Executor
// stdin/stdout JSON 行协议（08沙箱规范 §3）。
// 在独立 OS 子进程中运行。执行 shell 命令并返回结果。
package main

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"time"
)

// ─── IPC 消息类型 (Daemon → Plugin, stdin) ───

type ipcMessage struct {
	Type         string   `json:"type"`
	Config       map[string]interface{} `json:"config,omitempty"`
	Capabilities []string `json:"capabilities,omitempty"`
	Workspace    string   `json:"workspace,omitempty"`
	Tmp          string   `json:"tmp,omitempty"`
	ActionID     string   `json:"action_id,omitempty"`
	ActionType   string   `json:"action_type,omitempty"`
	Target       string   `json:"target,omitempty"`
	Params       map[string]interface{} `json:"params,omitempty"`
	TimeoutMs    int      `json:"timeout_ms,omitempty"`
	Reason       string   `json:"reason,omitempty"`
}

// ─── IPC 消息类型 (Plugin → Daemon, stdout) ───

type resultMessage struct {
	Type      string `json:"type"`
	ActionID  string `json:"action_id"`
	Status    string `json:"status"`
	Output    string `json:"output,omitempty"`
	Error     string `json:"error,omitempty"`
	CostMs    int    `json:"cost_ms"`
}

type errorMessage struct {
	Type    string `json:"type"`
	Code    string `json:"code"`
	Message string `json:"message"`
}

func main() {
	scanner := bufio.NewScanner(os.Stdin)
	// 增大 scanner buffer（JSON 行协议 < 4KB，留余量 64KB）
	scanner.Buffer(make([]byte, 65536), 65536)

	for scanner.Scan() {
		line := scanner.Bytes()
		var msg ipcMessage
		if err := json.Unmarshal(line, &msg); err != nil {
			writeError("invalid_message", fmt.Sprintf("cannot parse message: %v", err))
			continue
		}

		switch msg.Type {
		case "init":
			// 初始化——验证必备字段后开始工作
			if msg.Workspace == "" {
				writeError("init_failed", "missing workspace path")
				os.Exit(1)
			}
			// 静默确认——init 成功不产生输出消息
		case "execute":
			handleExecute(msg)
		case "shutdown":
			os.Exit(0)
		default:
			writeError("invalid_message", fmt.Sprintf("unknown message type: %s", msg.Type))
		}
	}

	if err := scanner.Err(); err != nil {
		writeError("internal_error", fmt.Sprintf("stdin read error: %v", err))
		os.Exit(1)
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
		// 从 params 中取
		if p, ok := msg.Params["command"]; ok {
			cmdStr = fmt.Sprint(p)
		}
	}
	if cmdStr == "" {
		writeResult(msg.ActionID, "failure", "", "no command specified", int(time.Since(start).Milliseconds()))
		return
	}

	timeout := time.Duration(msg.TimeoutMs) * time.Millisecond
	if timeout <= 0 {
		timeout = 30 * time.Second
	}

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
	_ = timeout // timeout enforced by PluginRunner via context
}

func writeResult(actionID, status, output, errStr string, costMs int) {
	writeJSON(resultMessage{
		Type:     "result",
		ActionID: actionID,
		Status:   status,
		Output:   output,
		Error:    errStr,
		CostMs:   costMs,
	})
}

func writeError(code, message string) {
	writeJSON(errorMessage{
		Type:    "error",
		Code:    code,
		Message: message,
	})
}

func writeJSON(v interface{}) {
	data, _ := json.Marshal(v)
	fmt.Println(string(data))
}
