//go:build darwin

// bypass_darwin_test.go——TC-RT-001c（12 清单 F 节——TestRuntime_Bypass_SyscallDenied_Darwin）：
// 直接系统调用旁路拒绝（macOS Seatbelt 族）：同上断言（Seatbelt 纵深防御语义——
// 06 §1.3 macOS 行「约定级」标注 R-1571，弃用 API 诚实标注 R-1532）。
// 先红=W1；转绿=W5 任务 5.3（Seatbelt 收敛为受限档 Provider——本文件传真实 Provider）。
// runBypassProbe 单态助手 darwin 独占（定义面=使用面——U1000 按构建上下文判定）。
package runtime

import (
	"context"
	"testing"
)

func TestRuntime_Bypass_SyscallDenied_Darwin(t *testing.T) {
	// 探针工作区/临时目录=测试临时目录（t.TempDir() 自动清理——边界允许面最小化）
	ws := t.TempDir()
	tmp := t.TempDir()
	runBypassProbe(t, "TC-RT-001c", NewDarwinSeatbeltProvider(ws, tmp))
}

// runBypassProbe 单态断言（darwin 独占——TC 原文逐字实现，禁止缩水）：
// p=nil=该平台 Provider 未收敛（登记先红——R-1452 合法形态）；
// p 非 nil=真实旁路断言：沙箱内进程绕过能力代理直接 open（工作区外路径）/
// connect（出站）→断言被 OS 边界拒绝（非代理拒绝误计——BypassCounters 分离计数）。
func runBypassProbe(t *testing.T, tc string, p Provider) {
	t.Helper()
	if p == nil {
		t.Skipf("%s 先红（W1 注册，R-1452 合法先红形态）：本平台受限档 Provider 未收敛——转绿=任务 5.1/5.2/5.3", tc)
	}
	ctx := context.Background()

	// 注册+取回（RegisterChecked 骨架探测——R-1468；AcquireProviderForTier=生产解析路径同源）
	reg := NewProviderRegistry()
	if err := reg.RegisterChecked(p); err != nil {
		t.Fatalf("%s：Provider 注册失败（骨架纪律）: %v", tc, err)
	}
	got, err := reg.AcquireProviderForTier("T1")
	if err != nil {
		t.Fatalf("%s：取回失败: %v", tc, err)
	}
	if err := got.Prepare(ctx, RuntimePlan{PlanID: tc + "-probe", Tier: TierRestricted}); err != nil {
		t.Fatalf("%s：Prepare 失败: %v", tc, err)
	}

	// 租约+D-5 定序（Start 物化边界→Precheck 探针验证→Execute 唯一合法态）
	h, err := got.Acquire(ctx, LeaseRequest{GoalID: "bypass-probe", ActionID: tc})
	if err != nil {
		t.Fatalf("%s：Acquire 失败: %v", tc, err)
	}
	guard := NewHandleGuard(h)
	if err := guard.Start(ctx); err != nil {
		t.Fatalf("%s：Start 失败: %v", tc, err)
	}
	if err := guard.Precheck(ctx); err != nil {
		t.Fatalf("%s：Precheck 边界验证失败（边界建立但未生效）: %v", tc, err)
	}
	runBypassProbeMatrix(t, tc, guard)
}
