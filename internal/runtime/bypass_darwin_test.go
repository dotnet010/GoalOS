//go:build darwin

// bypass_darwin_test.go——TC-RT-001c（12 清单 F 节——TestRuntime_Bypass_SyscallDenied_Darwin）：
// 直接系统调用旁路拒绝（macOS Seatbelt 族）：同上断言（Seatbelt 纵深防御语义——
// 06 §1.3 macOS 行「约定级」标注 R-1571，弃用 API 诚实标注 R-1532）。
// 先红=W1；转绿=W5-6（任务 5.3 Seatbelt 收敛为受限档 Provider）。
package runtime

import "testing"

func TestRuntime_Bypass_SyscallDenied_Darwin(t *testing.T) {
	runBypassProbe(t, "TC-RT-001c")
}
