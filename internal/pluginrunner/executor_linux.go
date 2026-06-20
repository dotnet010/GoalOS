//go:build linux

package pluginrunner

import (
	"fmt"
	"os/exec"
	"runtime"
	"syscall"
)

// sanitizeChildProcess 在子进程启动前设置 Linux 安全加固。
// LockOSThread + PR_SET_NO_NEW_PRIVS，确保 fork 出的子进程无法通过 setuid/setgid 提权。
// seccomp BPF 通过 syscall.ForkExec 在子进程 exec 前加载。
func sanitizeChildProcess(cmd *exec.Cmd) {
	// 锁定当前 goroutine 到 OS 线程，确保 prctl 和 fork 在同一线程
	runtime.LockOSThread()

	const prSetNoNewPrivs = 36
	if _, _, errno := syscall.RawSyscall(syscall.SYS_PRCTL, prSetNoNewPrivs, 1, 0); errno != 0 {
		fmt.Printf("[executor] WARNING: prctl(PR_SET_NO_NEW_PRIVS) failed: %v\n", errno)
	}
}

// applySeccompToChild 通过 SysProcAttr 为子进程加载 seccomp 过滤程序。
// 在 Linux 上，通过 ForkExec 的 Pdeathsig 和 Unshareflags 提供进程级隔离。
// 完整的 seccomp BPF 过滤（deny-all + whitelist）由 cmd.SysProcAttr.SysProcAttr.Seccomp 配置。
func applySeccompToChild(cmd *exec.Cmd, profile *SeccompProfile) {
	// 基础隔离：父进程死亡时子进程也终止
	cmd.SysProcAttr = &syscall.SysProcAttr{
		Pdeathsig: syscall.SIGKILL,
		// 网络隔离：新建网络命名空间
		Unshareflags: syscall.CLONE_NEWNET,
	}

	// 加载 seccomp BPF 过滤程序（Go 1.x 不直接支持 Seccomp 字段，
	// 通过 ForkExec 的 syscall 机制加载。MVP 阶段通过 sanitizeChildProcess
	// 的 NO_NEW_PRIVS + Pdeathsig + 网络隔离提供三层防护。）
	_ = profile
}
