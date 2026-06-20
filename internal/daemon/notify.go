// Package daemon — OS 原生通知。
// macOS: osascript display notification。Linux: notify-send。
// Goal 完成时发送系统通知。
//
// 设计依据：05 架构文档 §9、R33。
package daemon

import (
	"fmt"
	"os/exec"
	"runtime"
)

// Notify 发送 OS 原生通知。
// 通知失败不影响 Goal 完成状态——静默失败。
func Notify(title, message string) error {
	switch runtime.GOOS {
	case "darwin":
		return notifyDarwin(title, message)
	case "linux":
		return notifyLinux(title, message)
	default:
		return fmt.Errorf("notify: 不支持 %s", runtime.GOOS)
	}
}

// notifyDarwin 使用 osascript 发送 macOS 通知。
func notifyDarwin(title, message string) error {
	script := fmt.Sprintf(
		`display notification "%s" with title "%s" sound name "default"`,
		escapeAppleScript(message),
		escapeAppleScript(title),
	)
	return exec.Command("osascript", "-e", script).Run()
}

// notifyLinux 使用 notify-send 发送 Linux 通知。
func notifyLinux(title, message string) error {
	return exec.Command("notify-send", title, message).Run()
}

// escapeAppleScript 转义字符串中的双引号和反斜杠。
func escapeAppleScript(s string) string {
	result := ""
	for _, c := range s {
		switch c {
		case '\\':
			result += "\\\\"
		case '"':
			result += "\\\""
		default:
			result += string(c)
		}
	}
	return result
}

// NotifyGoalCompleted 发送 Goal 完成通知。
func NotifyGoalCompleted(goalTitle, artifactPath string) {
	msg := fmt.Sprintf("产出物: %s", artifactPath)
	if err := Notify("目标已完成 — "+goalTitle, msg); err != nil {
		// 通知失败静默——不影响 Goal 完成状态
	}
}
