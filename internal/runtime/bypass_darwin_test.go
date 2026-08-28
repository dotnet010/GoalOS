//go:build darwin

// bypass_darwin_test.go——TC-RT-001c（12 清单 F 节——TestRuntime_Bypass_SyscallDenied_Darwin）：
// 直接系统调用旁路拒绝（macOS Seatbelt 族）：同上断言（Seatbelt 纵深防御语义——
// 06 §1.3 macOS 行「约定级」标注 R-1571，弃用 API 诚实标注 R-1532）。
// 先红=W1；转绿=W5 任务 5.3（Seatbelt 收敛为受限档 Provider——本文件传真实 Provider）。
package runtime

import "testing"

func TestRuntime_Bypass_SyscallDenied_Darwin(t *testing.T) {
	// 探针工作区/临时目录=测试临时目录（t.TempDir() 自动清理——边界允许面最小化）
	ws := t.TempDir()
	tmp := t.TempDir()
	runBypassProbe(t, "TC-RT-001c", NewDarwinSeatbeltProvider(ws, tmp))
}
