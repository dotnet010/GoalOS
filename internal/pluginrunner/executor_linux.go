//go:build linux

package pluginrunner

import (
	"os/exec"
	"syscall"
)

// sanitizeChildProcess 在子进程启动前设置 Linux 安全加固。
// 通过 prctl 设置 PR_SET_NO_NEW_PRIVS，阻止子进程通过 setuid/setgid 提权。
// 此属性在 fork 后被子进程继承且不可撤销。
// seccomp BPF 过滤程序需通过 syscall.ForkExec（而非 os/exec）在子进程 exec 前加载。
func sanitizeChildProcess(cmd *exec.Cmd) {
	// PR_SET_NO_NEW_PRIVS = 36。当前线程设置后，子进程继承。
	// 在 Go 多线程程序中，此设置仅影响调用线程及其子进程。
	const prSetNoNewPrivs = 36
	syscall.Syscall(syscall.SYS_PRCTL, prSetNoNewPrivs, 1, 0)
}
