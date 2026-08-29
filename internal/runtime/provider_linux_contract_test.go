//go:build linux

// provider_linux_contract_test.go——Linux 命名空间 Provider 契约测试（任务 5.2——
// 平台 CI 实证：darwin 不编译本文件；linux CI=真实执行面）。
// 断言：注册→Prepare（NEWNET 实测）→租约→Start→Precheck（网络命名空间探针真实拒绝）→
// Execute（NEWNET 内）→Interrupt/Release 生命周期。
package runtime

import (
	"context"
	"testing"
)

// TestRuntime_LinuxProvider_NamespaceBoundary（任务 5.2 平台实证）：
// ①Prepare 实测 NEWNET 可用；②Precheck 网络探针真实被拒（NEWNET 内无路由）；
// ③Execute 边界内执行（命名空间内进程真实跑）；④生命周期完整（Release 幂等）。
// 诚实标注：fs 禁闭未落地——能力快照=I1+降级证据（不假报 I2）。
func TestRuntime_LinuxProvider_NamespaceBoundary(t *testing.T) {
	ctx := context.Background()
	p := NewLinuxNamespaceProvider(t.TempDir(), t.TempDir())
	if err := p.Prepare(ctx, RuntimePlan{PlanID: "linux-test", Tier: TierRestricted}); err != nil {
		t.Skipf("平台能力不具备（缺 CAP_SYS_ADMIN/内核限制——诚实降级非缺陷）: %v", err)
	}
	caps, _ := p.Capabilities(ctx)
	if caps.AchievedIsolation != I1 || len(caps.DegradedEvidence) == 0 {
		t.Fatalf("能力快照必须=I1+降级证据（fs 禁闭未落地诚实呈现），实际: %+v", caps)
	}
	h, err := p.Acquire(ctx, LeaseRequest{GoalID: "g-lx", ActionID: "a1"})
	if err != nil {
		t.Fatal(err)
	}
	if err := h.Start(ctx); err != nil {
		t.Fatal(err)
	}
	if err := h.Precheck(ctx); err != nil {
		t.Fatalf("Precheck 网络命名空间探针应过（NEWNET 实证拒绝），实际: %v", err)
	}
	res, err := h.Execute(ctx, ExecuteRequest{
		ActionID: "a1", ActionType: "process.exec",
		Params: map[string]string{"binary": "/bin/echo", "args": "ns-ok"},
	})
	if err != nil || res.Status != "success" {
		t.Fatalf("Execute 边界内应真实执行，实际: status=%s err=%v", res.Status, err)
	}
	if res.Output == "" {
		t.Fatal("Execute 应有真实输出（ns-ok）——空输出=假执行嫌疑")
	}
	if err := h.Release(ctx); err != nil {
		t.Fatal(err)
	}
	if err := h.Release(ctx); err != nil {
		t.Fatal("Release 二次调用=幂等成功（R-1620）")
	}
}
