//go:build darwin

package pluginrunner

import "os/exec"

// sanitizeChildProcess 在子进程启动前应用平台安全加固（macOS）。
// macOS 使用 Seatbelt (sandbox-exec) 而非 seccomp。
// 当前设置为基本防护；完整 sandbox 需创建 .sb profile 文件。
func sanitizeChildProcess(cmd *exec.Cmd) {
	// macOS 基本防护：子进程继承 daemon 的沙箱（如果 daemon 自身在沙箱中运行）
	// 完整实现：使用 sandbox-exec 工具包装子进程
}
