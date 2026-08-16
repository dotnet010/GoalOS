// 契约测试：I4-I5 层级契约（R-1078/R-1079——会议 #193 威胁模型升级）。
//
// 断言来源: R-1078（L4-L5 隔离层级扩展——gVisor systrap 无 KVM 依赖=信创 4.19 可跑
//   L4；Windows Sandbox .wsb；macOS 诚实标注维持）；R-1079（L5 一次性执行环境——
//   实现推迟 v0.4.0 诚实标注）。
//
// 当前契约形态: 后端骨架（runsc/sandbox-exec 子进程调用归 3.19/5.3 完成态）。
//   骨架期 fail-closed 契约（R-1468）：Execute 返回描述性错误（不伪装成功）；
//   CompiledProfile 必须经 Compile 显式产出（R-1106 零值非法）。L5 实现推迟
//   v0.4.0=诚实标注（R-1079）——I5 声明可编译但无执行路径（fail-closed）。
//   平台后端经 platformBackend 分平台选择（linux→gVisor；darwin→Seatbelt；
//   其余→MXC stub——均实现同一 fail-closed 契约）。
//
// 纪律: 行为断言——禁止源码文本断言；逐条 t.Error 非 FailNow。

package sandbox_test

import (
	"context"
	"testing"

	"github.com/goalos/goalos/internal/sandbox"
)

// i4Profile 构造 I4 级已编译 profile（Compile 显式构造——R-1106 零值非法）。
func i4Profile(t *testing.T) sandbox.CompiledProfile {
	t.Helper()
	p, err := sandbox.Compile(&sandbox.RawProfile{Isolation: "I4"}, sandbox.PlatformLinux)
	if err != nil {
		t.Fatalf("前置: I4 profile 编译失败: %v", err)
	}
	return *p
}

// TestSandbox_GVisorTier_DetectAndRoute — 层级探测与路由骨架契约（R-1078）。
// 断言：I4 profile 可编译（层级声明合法）；骨架期 Execute fail-closed
// （后端子进程调用未实现=不伪装执行成功）；Close 幂等。
func TestSandbox_GVisorTier_DetectAndRoute(t *testing.T) {
	profile := i4Profile(t)
	// MUST 1（R-1106/R-1012）: I4 经 Compile 显式产出（Compiled 标记=true——
	// 零值非法化的合法路径唯一）。
	if !profile.Compiled() {
		t.Error("MUST 1（R-1106）: Compile 产出的 profile Compiled()=false——合法路径断裂")
	}
	if profile.IsolationLevel() != 4 {
		t.Errorf("MUST 1（R-1078）: I4 编译后隔离等级=%d，必须为 4——层级声明失真", profile.IsolationLevel())
	}

	backend := platformBackend(t)
	// MUST 2（R-1468）: 骨架期 Execute 必须返回错误——后端调用未实现不伪装成功。
	if _, err := backend.Execute(context.Background(), &sandbox.CommandSpec{Path: "/bin/true"}, profile); err == nil {
		t.Error("MUST 2（R-1078/R-1468）: 骨架期 Execute 返回 nil error——伪装执行成功违约")
	}
	// MUST 3（生命周期）: Close 幂等 nil。
	if err := backend.Close(); err != nil {
		t.Errorf("MUST 3（R-1078）: Close 返回错误: %v", err)
	}
}

// TestSandbox_L5Disposable_HonestDeferral — L5 一次性执行环境诚实标注（R-1079）。
// 断言：I5 声明可编译（层级词汇合法）；但无 L5 执行路径——Execute fail-closed
// （实现推迟 v0.4.0 的诚实标注：不伪装支持）。
func TestSandbox_L5Disposable_HonestDeferral(t *testing.T) {
	p, err := sandbox.Compile(&sandbox.RawProfile{Isolation: "I5"}, sandbox.PlatformLinux)
	if err != nil {
		t.Fatalf("前置: I5 profile 编译失败: %v", err)
	}
	// MUST 1（R-1079）: I5 层级声明合法（词汇存在——一次性执行环境的接口占位）。
	if p.IsolationLevel() != 5 {
		t.Errorf("MUST 1（R-1079）: I5 编译后隔离等级=%d，必须为 5——层级声明失真", p.IsolationLevel())
	}

	// MUST 2（R-1079 诚实标注）: 无 L5 执行路径——后端 fail-closed（不伪装支持）。
	backend := platformBackend(t)
	if _, err := backend.Execute(context.Background(), &sandbox.CommandSpec{Path: "/bin/true"}, *p); err == nil {
		t.Error("MUST 2（R-1079）: L5 执行返回 nil error——实现推迟 v0.4.0 期间伪装支持违约（诚实标注要求 fail-closed）")
	}
	// MUST 3（R-1079）: 重复调用同样 fail-closed——无偶发假成功路径。
	if _, err := backend.Execute(context.Background(), &sandbox.CommandSpec{Path: "/bin/true"}, *p); err == nil {
		t.Error("MUST 3（R-1079）: L5 重复执行返回 nil error——fail-closed 不稳定")
	}
}
