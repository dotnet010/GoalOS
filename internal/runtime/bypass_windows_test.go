//go:build windows

// bypass_windows_test.go——TC-RT-001a（12 清单 F 节）：Windows 受限档旁路实证。
// 2026-09-06 起改挂生产基座 WinAC（R-1648——AppContainer 承载）+双态断言
//（R-1452 延伸——承载能力在场=全矩阵；缺席=fail-closed 实证绿）。
// WinAC 无需隔离 runner 窗口（非管理员态本机已实证——原 GOALOS_AGENTBOX_CI 门禁
// 随 agentbox 族全量移除而退役，2026-09-08）：windows-daily 每次运行=真实出数
//（R-1654 CI smoke 承载点）。
package runtime

import (
	"testing"
)

func TestRuntime_Bypass_SyscallDenied_Windows(t *testing.T) {
	runBypassProbeDual(t, "TC-RT-001a", NewWinACProvider(t.TempDir(), t.TempDir(), nil))
}
