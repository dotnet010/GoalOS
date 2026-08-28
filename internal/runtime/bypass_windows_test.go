//go:build windows

// bypass_windows_test.go——TC-RT-001a（12 清单 F 节——TestRuntime_Bypass_SyscallDenied_Windows）：
// 直接系统调用旁路拒绝（Windows 受限令牌族）：沙箱内进程绕过能力代理直接 open/connect，
// 断言被 OS 边界拒绝（受限令牌+Job Object+ACL——非代理拒绝误计）。
// 先红=W1（注册表空）；转绿=W5-6（任务 5.1 agentbox 收敛为受限档 Provider）。
package runtime

import "testing"

func TestRuntime_Bypass_SyscallDenied_Windows(t *testing.T) {
	runBypassProbe(t, "TC-RT-001a")
}
