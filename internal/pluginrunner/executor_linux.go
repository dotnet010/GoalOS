//go:build linux

// Package pluginrunner — Linux 子进程安全加固（v0.3.0）。
// CLONE_NEWNET 网络隔离 + Pdeathsig 父进程死亡时自动清理。
// 设计依据：08 沙箱规范 §4、会议 #63 Linus 方案、R-863。
// 注：v0.3.0 fix C9 的 daemon 侧 verifySeccompLoaded（/proc/<pid>/status 读 Seccomp
// 字段）已随双引擎收敛（R-1664——模式 B 子进程自加载 seccomp+TSYNC，Precheck
// 实证探测取代静态验证）孤儿化——零调用方，2026-09-06 删除（staticcheck U1000）。
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
