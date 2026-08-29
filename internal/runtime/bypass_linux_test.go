//go:build linux

// bypass_linux_test.go——TC-RT-001b（12 清单 F 节）：Linux 受限档 Provider（seccomp+ns+
// cgroup）收敛前=先红（nil Provider——R-1452 合法形态）；转绿=任务 5.2。
package runtime

import "testing"

func TestRuntime_Bypass_SyscallDenied_Linux(t *testing.T) {
	// 转绿窗口（任务 5.2 收敛落地——agentbox 承载）：Provider 真实注册
	runBypassProbe(t, "TC-RT-001b", NewAgentboxProvider(t.TempDir(), t.TempDir(), "linux"))
}
