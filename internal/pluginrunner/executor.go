// Package pluginrunner — IPC 子进程执行器。
// os/exec 启动独立子进程。stdin/stdout JSON 行协议。
// 子进程崩溃不影响 daemon。超时→SIGTERM→SIGKILL。
//
// 设计依据：08 沙箱与 IPC 规范 §2-§3、R137、R197。
package pluginrunner

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"syscall"
	"time"
)

// ExecConfig 子进程执行配置。
type ExecConfig struct {
	BinaryPath string
	Args       []string
	WorkDir    string
	TmpDir     string
	Timeout    time.Duration
}

// ExecResult 子进程执行结果。
type ExecResult struct {
	ActionID   string `json:"action_id"`
	Status     string `json:"status"`
	Output     string `json:"output"`
	Error      string `json:"error"`
	DurationMs int    `json:"cost_ms"`
}

// Execute 启动子进程，通过 stdin/stdout JSON 行协议通信。
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

	sanitizeChildProcess(cmd)
	applySeccompToChild(cmd, DefaultSeccomp())

	if cfg.WorkDir != "" {
		if err := os.MkdirAll(cfg.WorkDir, 0755); err != nil {
			return nil, fmt.Errorf("executor: mkdir: %w", err)
		}
	}

	startTime := time.Now()
	if err := cmd.Start(); err != nil {
		return nil, fmt.Errorf("executor: start: %w", err)
	}

	// 发送 init
	writeJSON(stdin, map[string]interface{}{
		"type":         "init",
		"capabilities": action.RequiredCapabilities,
		"workspace":    cfg.WorkDir,
		"tmp":          cfg.TmpDir,
	})

	// 在发送 execute 之前启动 stdout 读取 goroutine
	resultCh := make(chan *ExecResult, 1)
	errCh := make(chan error, 1)
	go readStdout(stdout, resultCh, errCh)

	// 发送 execute（不含 token）
	writeJSON(stdin, map[string]interface{}{
		"type":        "execute",
		"action_id":   action.ActionID,
		"action_type": action.ActionType,
		"target":      action.Target,
		"params":      action.Params,
		"timeout_ms":  cfg.Timeout.Milliseconds(),
	})

	// 等待结果
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

	// 发送 shutdown，等待退出
	writeJSON(stdin, map[string]interface{}{
		"type":   "shutdown",
		"reason": "completed",
	})
	stdin.Close()
	cmd.Wait()

	result.DurationMs = int(time.Since(startTime).Milliseconds())
	return result, nil
}

// readStdout 从 stdout 读取子进程返回的 ResultMessage。
func readStdout(stdout io.Reader, resultCh chan<- *ExecResult, errCh chan<- error) {
	scanner := bufio.NewScanner(stdout)
	scanner.Buffer(make([]byte, 65536), 65536)
	for scanner.Scan() {
		var msg map[string]interface{}
		if err := json.Unmarshal(scanner.Bytes(), &msg); err != nil {
			continue
		}
		msgType, _ := msg["type"].(string)
		switch msgType {
		case "result":
			r := &ExecResult{ActionID: str(msg["action_id"]), Status: str(msg["status"])}
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
