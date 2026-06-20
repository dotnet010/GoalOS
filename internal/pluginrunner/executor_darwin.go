//go:build darwin

package pluginrunner

import (
	"os/exec"
	"syscall"
)

// sanitizeChildProcess 在子进程启动前设置 macOS 安全加固。
// macOS 使用 Seatbelt (sandbox-exec) 而非 Linux seccomp。
// MVP 阶段：子进程进入独立进程组，防止终端信号传播。
// 完整 sandbox 需创建 .sb profile 文件并使用 sandbox-exec 包装（v2）。
func sanitizeChildProcess(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{
		Setpgid: true,
	}
}
