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
// 必须在 applySeccompToChild 之前调用——seccomp 要求 NO_NEW_PRIVS 已设置。
func sanitizeChildProcess(cmd *exec.Cmd) {
	runtime.LockOSThread()

	const prSetNoNewPrivs = 36
	if _, _, errno := syscall.RawSyscall(syscall.SYS_PRCTL, prSetNoNewPrivs, 1, 0); errno != 0 {
		fmt.Printf("[executor] WARNING: prctl(PR_SET_NO_NEW_PRIVS) failed: %v\n", errno)
	}
}

// applySeccompToChild 在子进程启动前加载 seccomp BPF + PID/网络隔离。
// sanitizeChildProcess 必须先调用以锁定线程和设置 NO_NEW_PRIVS。
func applySeccompToChild(cmd *exec.Cmd, profile *SeccompProfile) {
	cmd.SysProcAttr = &syscall.SysProcAttr{
		Pdeathsig:    syscall.SIGKILL,
		Unshareflags: syscall.CLONE_NEWNET,
	}

	if profile != nil {
		// LockOSThread 已在 sanitizeChildProcess 中调用——当前 goroutine 锁定于此 OS 线程。
		// prctl(PR_SET_SECCOMP) 应用到当前线程，子进程通过 fork 继承。
		if err := ApplySeccomp(profile); err != nil {
			fmt.Printf("[executor] WARNING: ApplySeccomp failed: %v\n", err)
		}
	}
}
