//go:build windows

// bypass_windows_test.go——TC-RT-001a（12 清单 F 节）：Windows 受限档 Provider（agentbox）
// 收敛前=先红（nil Provider——R-1452 合法形态）；转绿=任务 5.1。
package runtime

import "testing"

func TestRuntime_Bypass_SyscallDenied_Windows(t *testing.T) {
	runBypassProbe(t, "TC-RT-001a", nil)
}
