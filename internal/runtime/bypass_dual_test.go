//go:build linux || windows

// bypass_dual_test.go——TC-RT-001a/b 双态断言助手（定义面=使用面：linux+windows
// 两侧引用——staticcheck U1000 按构建上下文判定，2026-09-06 CI 实证事故后
// 从 shared 拆出；darwin 走单态助手（bypass_darwin_test.go）不进本文件）。
package runtime

import (
	"context"
	"strings"
	"testing"
)

// runBypassProbeDual 双态断言（2026-08-31 Ubuntu 24.04 实机裁决——R-1452 延伸）：
// 平台有能力承载边界 → 走全探针矩阵（断言强度不降）；
// 平台无能力（AppArmor 限 userns 族/受限机制缺席）→ Precheck 必须 fail-closed
// （「边界失效」证据文本）——裸跑被执行=不可能到绿。两态都绿=安全性质成立；
// 边界静默缺席=红。
func runBypassProbeDual(t *testing.T, tc string, p Provider) {
	t.Helper()
	if p == nil {
		t.Skipf("%s 先红（W1 注册，R-1452 合法先红形态）：本平台受限档 Provider 未收敛", tc)
	}
	ctx := context.Background()
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
	h, err := got.Acquire(ctx, LeaseRequest{GoalID: "bypass-probe", ActionID: tc})
	if err != nil {
		t.Fatalf("%s：Acquire 失败: %v", tc, err)
	}
	guard := NewHandleGuard(h)
	if err := guard.Start(ctx); err != nil {
		t.Fatalf("%s：Start 失败: %v", tc, err)
	}
	if err := guard.Precheck(ctx); err != nil {
		if strings.Contains(err.Error(), "边界失效") {
			t.Logf("%s：平台无受限档承载能力——fail-closed 实证（Precheck 拒绝=边界缺席诚实暴露，裸跑未发生）：%v", tc, err)
			return
		}
		t.Fatalf("%s：Precheck 失败（非边界失效签名）: %v", tc, err)
	}
	t.Logf("%s：Precheck 通过——平台有承载能力，进入全探针矩阵", tc)
	runBypassProbeMatrix(t, tc, guard)
}
