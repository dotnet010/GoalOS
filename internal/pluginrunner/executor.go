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
	"os/exec"
	"syscall"
	"time"
)

// ExecConfig 是子进程执行配置。
type ExecConfig struct {
	BinaryPath string        // 可执行文件路径
	Args       []string      // 命令行参数
	WorkDir    string        // 工作目录（~/Goals/<目标名>/）
	TmpDir     string        // 临时目录（/tmp/goalos/<action_id>/）
	Timeout    time.Duration // 执行超时
}

// ExecResult 是子进程执行结果。
type ExecResult struct {
	ActionID  string `json:"action_id"`
	Status    string `json:"status"`    // "success" | "failure"
	Output    string `json:"output"`    // 人类可读输出
	Error     string `json:"error"`     // 错误描述
	DurationMs int  `json:"cost_ms"`    // 执行耗时（毫秒）
}

// Execute 启动子进程并执行 Action。
// 通过 stdin/stdout JSON 行协议与子进程通信。
// 超时→SIGTERM→5s→SIGKILL。子进程崩溃不影响 daemon。
func Execute(cfg ExecConfig, action ActionRequest) (*ExecResult, error) {
	cmd := exec.Command(cfg.BinaryPath, cfg.Args...)
	if cfg.WorkDir != "" {
		cmd.Dir = cfg.WorkDir
	}
	// 设置子进程环境
	cmd.Env = []string{
		"GOALOS_WORKSPACE=" + cfg.WorkDir,
		"GOALOS_TMP=" + cfg.TmpDir,
	}

	// 获取 stdin/stdout 管道
	stdin, err := cmd.StdinPipe()
	if err != nil {
		return nil, fmt.Errorf("executor: stdin pipe 失败: %w", err)
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return nil, fmt.Errorf("executor: stdout pipe 失败: %w", err)
	}

	// 启动子进程
	startTime := time.Now()
	if err := cmd.Start(); err != nil {
		return nil, fmt.Errorf("executor: 启动子进程失败: %w", err)
	}

	// 发送 InitMessage
	initMsg := map[string]interface{}{
		"type":         "init",
		"config":       map[string]interface{}{},
		"capabilities": action.RequiredCapabilities,
		"workspace":    cfg.WorkDir,
		"tmp":          cfg.TmpDir,
	}
	if err := writeJSON(stdin, initMsg); err != nil {
		cmd.Process.Kill()
		return nil, fmt.Errorf("executor: 发送 init 失败: %w", err)
	}

	// 发送 ExecuteMessage（不含 token——R226 daemon 侧验证）
	execMsg := map[string]interface{}{
		"type":        "execute",
		"action_id":   action.ActionID,
		"action_type": action.ActionType,
		"target":      action.Target,
		"params":      action.Params,
		"timeout_ms":  cfg.Timeout.Milliseconds(),
	}
	if err := writeJSON(stdin, execMsg); err != nil {
		cmd.Process.Kill()
		return nil, fmt.Errorf("executor: 发送 execute 失败: %w", err)
	}

	// 读取子进程响应（带超时）
	result, err := readResult(cmd, stdout, cfg.Timeout)
	if err != nil {
		return nil, err
	}

	// 发送 ShutdownMessage
	shutdownMsg := map[string]interface{}{
		"type":   "shutdown",
		"reason": "completed",
	}
	writeJSON(stdin, shutdownMsg)
	stdin.Close()

	// 等待子进程退出
	cmd.Wait()

	result.DurationMs = int(time.Since(startTime).Milliseconds())
	return result, nil
}

// ActionRequest 是发送给子进程的 Action 描述。
type ActionRequest struct {
	ActionID             string   `json:"action_id"`
	ActionType           string   `json:"action_type"`
	Target               string   `json:"target"`
	Params               map[string]interface{} `json:"params"`
	RequiredCapabilities []string `json:"required_capabilities"`
}

// writeJSON 写入一条完整的 JSON 行消息到 stdin。
func writeJSON(w io.Writer, v interface{}) error {
	data, err := json.Marshal(v)
	if err != nil {
		return err
	}
	data = append(data, '\n')
	_, err = w.Write(data)
	return err
}

// readResult 从 stdout 读取子进程返回的 ResultMessage。
// 超时时发送 SIGTERM→SIGKILL。
func readResult(cmd *exec.Cmd, stdout io.Reader, timeout time.Duration) (*ExecResult, error) {
	done := make(chan *ExecResult, 1)
	errCh := make(chan error, 1)

	go func() {
		scanner := bufio.NewScanner(stdout)
		for scanner.Scan() {
			var msg map[string]interface{}
			if err := json.Unmarshal(scanner.Bytes(), &msg); err != nil {
				continue // 跳过非 JSON 行
			}
			msgType, _ := msg["type"].(string)
			if msgType == "result" {
				result := &ExecResult{
					ActionID: msg["action_id"].(string),
					Status:   msg["status"].(string),
				}
				if v, ok := msg["output"].(string); ok {
					result.Output = v
				}
				if v, ok := msg["error"].(string); ok {
					result.Error = v
				}
				done <- result
				return
			}
			if msgType == "error" {
				errCh <- fmt.Errorf("子进程错误: %v", msg["message"])
				return
			}
		}
		if err := scanner.Err(); err != nil {
			errCh <- fmt.Errorf("stdout 读取错误: %w", err)
		}
	}()

	select {
	case result := <-done:
		return result, nil
	case err := <-errCh:
		return nil, err
	case <-time.After(timeout):
		// 超时→SIGTERM
		cmd.Process.Signal(syscall.SIGTERM)
		// 5s 后如果仍未退出→SIGKILL
		select {
		case result := <-done:
			return result, nil
		case <-time.After(5 * time.Second):
			cmd.Process.Kill()
			return nil, fmt.Errorf("executor: 子进程超时（%v），已强制终止", timeout)
		}
	}
}
