//go:build windows

// Package pluginrunner — Windows 子进程安全加固（v0.1.2）。
// Windows 不支持 Linux seccomp/macOS Seatbelt。
// MVP 阶段：基础进程隔离，预留 Job Object 沙箱。
package pluginrunner

import (
	"os/exec"
)

// sanitizeChildProcess 在子进程启动前设置 Windows 安全加固。
// MVP 阶段：预留 Windows Job Object 沙箱（CreateJobObject/AssignProcessToJobObject）。
func sanitizeChildProcess(cmd *exec.Cmd) {
	// Windows 不支持 Setpgid/CLONE_NEWNET。
	// 未来版本将通过 Job Object 实现进程组隔离和资源限制。
}

// applySeccompToChild Windows 无 seccomp，stub 保留接口兼容性。
func applySeccompToChild(cmd *exec.Cmd, profile interface{}) {
	// no-op: seccomp is a Linux-only feature
}
