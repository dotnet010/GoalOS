//go:build windows

// bypass_windows_test.go——TC-RT-001a（12 清单 F 节）：Windows 受限档 Provider（agentbox
// 承载——Restricted Token+Job Object+Low IL+ACLs）转绿=任务 5.1 收敛落地（平台 CI 实证）。
package runtime

import "testing"

func TestRuntime_Bypass_SyscallDenied_Windows(t *testing.T) {
	// 转绿窗口（任务 5.1 收敛落地——agentbox 承载）：Provider 真实注册
	runBypassProbe(t, "TC-RT-001a", NewAgentboxProvider(t.TempDir(), t.TempDir(), "windows"))
}
