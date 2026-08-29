//go:build windows

// provider_windows_contract_test.go——Windows Job Object Provider 契约测试（任务 5.1——
// windows-daily CI 平台实证：darwin/linux 不编译本文件）。
// 断言：Prepare（Job API 空跑实测）→租约→Start（Job 创建）→Precheck（句柄在位）→
// Execute（进程入 Job）→Interrupt（组终止）→Release（KILL_ON_JOB_CLOSE 幂等）。
package runtime

import (
	"context"
	"testing"
)

// TestRuntime_WindowsProvider_JobBoundary（任务 5.1 平台实证）：
// ①Job API 实测可用；②能力快照诚实（I1+降级证据——Restricted Token/ACL 未落地不假报）；
// ③边界内真实执行（cmd.exe /c echo）；④Interrupt 组终止+Release 幂等。
func TestRuntime_WindowsProvider_JobBoundary(t *testing.T) {
	ctx := context.Background()
	p := NewWindowsJobProvider(t.TempDir(), t.TempDir())
	if err := p.Prepare(ctx, RuntimePlan{PlanID: "win-test", Tier: TierRestricted}); err != nil {
		t.Fatalf("Job Object API 实测应可用: %v", err)
	}
	caps, _ := p.Capabilities(ctx)
	if caps.AchievedIsolation != I1 || len(caps.DegradedEvidence) == 0 {
		t.Fatalf("能力快照必须=I1+降级证据（诚实呈现），实际: %+v", caps)
	}
	h, err := p.Acquire(ctx, LeaseRequest{GoalID: "g-win", ActionID: "a1"})
	if err != nil {
		t.Fatal(err)
	}
	if err := h.Start(ctx); err != nil {
		t.Fatal(err)
	}
	if err := h.Precheck(ctx); err != nil {
		t.Fatalf("Precheck Job 句柄在位断言应过，实际: %v", err)
	}
	res, err := h.Execute(ctx, ExecuteRequest{
		ActionID: "a1", ActionType: "process.exec",
		Params: map[string]string{"binary": "cmd.exe", "args": "/c echo job-ok"},
	})
	if err != nil || res.Status != "success" {
		t.Fatalf("Execute 边界内应真实执行，实际: status=%s err=%v", res.Status, err)
	}
	if err := h.Interrupt(ctx); err != nil {
		t.Fatal(err)
	}
	if err := h.Release(ctx); err != nil {
		t.Fatal(err)
	}
	if err := h.Release(ctx); err != nil {
		t.Fatal("Release 二次调用=幂等成功（R-1620）")
	}
}
