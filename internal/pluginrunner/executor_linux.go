//go:build linux

// Package pluginrunner — Linux 子进程安全加固（v0.1.0 会议 #63 Linus 方案）。
// seccomp 不再在父进程应用——由子进程在 init 阶段通过 pkg/seccomp 自加载。
// 父进程仅设置 namespace 隔离（CLONE_NEWNET）+ Pdeathsig。
package pluginrunner

import (
	"os/exec"
	"syscall"
)

// sanitizeChildProcess 在子进程启动前设置 Linux 安全加固。
// CLONE_NEWNET 网络命名空间隔离 + Pdeathsig 父进程死亡时自动清理。
func sanitizeChildProcess(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{
		Pdeathsig:    syscall.SIGKILL,
		Unshareflags: syscall.CLONE_NEWNET,
	}
}

// applySeccompToChild 已废弃（v0.1.0 会议 #63）。
// seccomp 由子进程在 init 阶段通过 pkg/seccomp.Apply() 自加载。
// 保留此函数以维持 executor_darwin.go 的接口兼容性。
func applySeccompToChild(cmd *exec.Cmd, profile interface{}) {
	// no-op: seccomp is self-applied by child process
}
