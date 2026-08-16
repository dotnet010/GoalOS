//go:build windows

// FD3 句柄继承契约（R-1086 / R-1383）——Windows 平台（本文件仅在 windows 编译）。
//
// 契约: Windows 上 FD3（fd3 const = 3）必须以 PROC_THREAD_ATTRIBUTE_HANDLE_LIST
// 显式白名单继承（syscall.SysProcAttr.AdditionalInheritedHandles），
// 禁止默认全量句柄继承（bInheritHandles=TRUE 的隐式继承是句柄注入面）。
// 锚定: 08 沙箱隔离与进程通信规范 FD3 协议（R-1086 平台策略）。
//
// 先红状态（阶段 3.5 测试先行闸口——用例先红）:
//   executor_windows.go 当前仅实现 Job Object 进程组隔离，未设置 HANDLE_LIST
//   白名单——FD3 继承未显式化。
//   本机 darwin 不编译本文件 → 按 C-11 纪律采用跳过守卫:
//   文件含真实断言体（断言 future API 契约）+ t.Skip（Windows 平台先红挂起）。
//
// 转绿任务: 3.27（C-2 表，v0.3.0 W3-4）——落地后移除 Skip，断言体即开始约束。
//
// 断言方式: 行为断言（SysProcAttr 白名单形态）——禁止读源码文本断言。
package pluginrunner

import (
	"os/exec"
	"syscall"
	"testing"
)

// TestWindows_FD3_HandleInheritance — Windows FD3 句柄显式继承白名单。
//
// MUST sanitizeChildProcess 为子进程设置 SysProcAttr（HANDLE_LIST 载体）
// MUST AdditionalInheritedHandles 白名单非空（FD3 控制通道可传递）
// MUST 白名单包含 FD3 句柄（fd3=3）
// MUST 白名单不含标准句柄 0/1/2（自动继承，不得重复声明）
func TestWindows_FD3_HandleInheritance(t *testing.T) {
	// C-11 跳过守卫: Windows 平台先红挂起——真实断言体在下方，转绿任务 3.27
	// 落地后移除本守卫即开始约束 FD3 句柄继承（v0.3.0 W3-4）。
	// 本机 darwin 不执行（文件仅 windows 编译）。
	t.Skip("Windows 平台先红挂起：v0.3.0 W3-4 随任务 3.27 转绿（R-1383）——本机 darwin 不执行")

	// ── 真实断言体（Windows 编译+运行才可达；转绿后恒绿）──
	cmd := exec.Command("cmd.exe", "/c", "exit /b 0")
	sanitizeChildProcess(cmd)

	sp, ok := any(cmd.SysProcAttr).(*syscall.SysProcAttr)
	if !ok || sp == nil {
		t.Fatalf("FD3 句柄继承未实现: SysProcAttr 为空——Windows 必须以 PROC_THREAD_ATTRIBUTE_HANDLE_LIST 显式白名单继承 FD3（R-1086）")
	}

	// 白名单语义: HANDLE_LIST 非空——FD3 控制通道必须可传递
	if len(sp.AdditionalInheritedHandles) == 0 {
		t.Errorf("HANDLE_LIST 白名单为空——FD3 控制通道无法传递（R-1086）")
	}
	// 白名单必须含 FD3，且不得混入标准句柄（0/1/2 自动继承，声明即注入面扩大）
	foundFD3 := false
	for _, h := range sp.AdditionalInheritedHandles {
		if uintptr(h) == uintptr(FD3) {
			foundFD3 = true
		}
		if uintptr(h) <= 2 {
			t.Errorf("HANDLE_LIST 白名单不得包含标准句柄 %d（0/1/2 自动继承）——白名单只承载 FD3（R-1086）", uintptr(h))
		}
	}
	if !foundFD3 {
		t.Errorf("HANDLE_LIST 白名单不含 FD3 句柄（fd3=%d），实际 %v——显式继承缺失（R-1086）", FD3, sp.AdditionalInheritedHandles)
	}
}
