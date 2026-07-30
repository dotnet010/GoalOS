//go:build linux

// Package pluginrunner — Linux 子进程安全加固（v0.3.0）。
// CLONE_NEWNET 网络隔离 + Pdeathsig + seccomp 加载验证。
// 设计依据：08 沙箱规范 §4、会议 #63 Linus 方案、R-863。
package pluginrunner

import (
	"fmt"
	"os"
	"os/exec"
	"strings"
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

// verifySeccompLoaded 验证子进程是否已加载 seccomp filter（v0.3.0 fix C9）。
// 读取 /proc/<pid>/status，检查 Seccomp 字段：
//   0 = disabled, 1 = strict, 2 = filter (SECCOMP_MODE_FILTER)
// 返回 nil 表示已加载，error 表示未加载或无法确认。
func verifySeccompLoaded(pid int) error {
	data, err := os.ReadFile(fmt.Sprintf("/proc/%d/status", pid))
	if err != nil {
		return fmt.Errorf("cannot read proc status for pid %d: %w", pid, err)
	}
	for _, line := range strings.Split(string(data), "\n") {
		if strings.HasPrefix(line, "Seccomp:") {
			fields := strings.Fields(line)
			if len(fields) >= 2 && fields[1] == "2" {
				return nil // SECCOMP_MODE_FILTER 已激活
			}
			return fmt.Errorf("seccomp not loaded for pid %d: status=%s", pid, fields[1])
		}
	}
	return fmt.Errorf("seccomp status not found for pid %d", pid)
}
