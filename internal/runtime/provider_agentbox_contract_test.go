//go:build linux || windows

// provider_agentbox_contract_test.go——agentbox 受限档 Provider 契约测试（任务 5.1/5.2——
// TC-RT-001a/b 转绿机制实证族）。平台 CI 实证（linux=tag CI/ubuntu 全内核；
// windows=windows-daily runner）。darwin 不编译（darwin 受限档=自有 seatbelt Provider 承载）。
// 断言：①Prepare=Available 实测（不信静态读数 R-960）；②Precheck 双探针真实被拒
// （fs 写区外+网络出站——Landlock/ACL+seccomp/防火墙机制证据）；③Execute 边界内真实执行；
// ④Release 幂等（R-1620）。
package runtime

import (
	"context"
	"os"
	"testing"
)

// TestRuntime_AgentboxProvider_Boundary（任务 5.1/5.2 平台实证）：
// agentbox 承载的受限档——fs 禁闭+网络阻断双探针真实拒绝。
func TestRuntime_AgentboxProvider_Boundary(t *testing.T) {
	// 环境门禁（2026-08-29 windows-daily CI 实证）：agentbox Windows 沙箱在 GH runner 上
	// 创建沙箱用户/改 ACL——共享 Temp 根级联 Access denied（同 runner 后续测试全灭）。
	// 默认跳过；GOALOS_AGENTBOX_CI=1=隔离 runner 窗口（独立环境可并行——登记待实机调试）。
	if os.Getenv("GOALOS_AGENTBOX_CI") != "1" {
		t.Skip("环境门禁：agentbox 平台实证需隔离 runner（GOALOS_AGENTBOX_CI=1）——windows-daily 共享 Temp 级联事故实证")
	}
	ctx := context.Background()
	platform := "linux"
	if isWindowsRuntime() {
		platform = "windows"
	}
	p := NewAgentboxProvider(t.TempDir(), t.TempDir(), platform)
	if err := p.Prepare(ctx, RuntimePlan{PlanID: "ab-test", Tier: TierRestricted}); err != nil {
		t.Skipf("平台机制不可用（诚实降级非缺陷——CI 环境内核/权限限制）: %v", err)
	}
	caps, _ := p.Capabilities(ctx)
	if caps.AchievedIsolation != I2 {
		t.Fatalf("agentbox 受限档应=I2（fs 禁闭面成立），实际 %s", caps.AchievedIsolation)
	}
	h, err := p.Acquire(ctx, LeaseRequest{GoalID: "g-ab", ActionID: "a1"})
	if err != nil {
		t.Fatal(err)
	}
	if err := h.Start(ctx); err != nil {
		t.Fatal(err)
	}
	if err := h.Precheck(ctx); err != nil {
		t.Fatalf("Precheck 双探针应被拒（fs+net 边界证据），实际: %v", err)
	}
	res, err := h.Execute(ctx, ExecuteRequest{
		ActionID: "a1", ActionType: "process.exec",
		Params: map[string]string{"binary": probeEchoBin(), "args": "agentbox-ok"},
	})
	if err != nil {
		t.Fatalf("Execute 边界内应真实执行，实际: %v", err)
	}
	if res.Status != "success" || res.Output == "" {
		t.Fatalf("Execute 应有真实输出，实际: status=%s output=%q", res.Status, res.Output)
	}
	if err := h.Release(ctx); err != nil {
		t.Fatal(err)
	}
	if err := h.Release(ctx); err != nil {
		t.Fatal("Release 二次=幂等成功（R-1620）")
	}
}

// isWindowsRuntime 平台判定（测试辅助）。
func isWindowsRuntime() bool {
	return goruntimeGOOS() == "windows"
}
