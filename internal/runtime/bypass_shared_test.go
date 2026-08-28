// bypass_shared_test.go——TC-RT-001 族共享探针（R-1452：SKIP=合法先红/FAIL=非法；
// R-571 测试先行；断言来源=12 清单 F 节 TC-RT-001a/b/c 行）。
// 旁路测试纪律（TC-RT-002 联动）：能力代理拒绝≠边界拒绝——分开计数（BypassCounters）。
package runtime

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// runBypassProbe 三平台共享：p=nil=该平台 Provider 未收敛（登记先红——R-1452 合法形态）；
// p 非 nil=真实旁路断言（转绿窗口——TC 原文逐字实现，禁止缩水）：
// 沙箱内进程绕过能力代理直接 open（工作区外路径）/connect（出站）→断言被 OS 边界拒绝
// （非代理拒绝误计——BypassCounters 分离计数：RecordProxyRefusal 不得提升 BypassPasses）。
func runBypassProbe(t *testing.T, tc string, p Provider) {
	t.Helper()
	if p == nil {
		t.Skipf("%s 先红（W1 注册，R-1452 合法先红形态）：本平台受限档 Provider 未收敛——转绿=任务 5.1/5.2/5.3", tc)
	}
	ctx := context.Background()
	counters := NewBypassCounters()

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

	// 探针组（Option B 语义——会议 #256）：写禁闭+敏感目录禁读+网络禁闭
	home, _ := os.UserHomeDir()
	probes := []struct{ name, binary, args string }{
		// 探针①：直接写工作区外路径（绕过能力代理——不经任何协议层）
		{"direct write outside workspace", "/usr/bin/touch", filepath.Join(home, ".goalos-bypass-probe-" + tc)},
		// 探针②：读敏感目录（读开放语义的显式收口面——密钥/凭证防线）
		{"sensitive dir read ~/.ssh", "/bin/cat", filepath.Join(home, ".ssh")},
		// 探针③：connect 出站（127.0.0.1:9 无监听——EPERM「Operation not permitted」与
		// ECONNREFUSED「Connection refused」文本可分辨，nc -v 强制输出）
		{"outbound connect 127.0.0.1:9", "/usr/bin/nc", "-v -w 1 127.0.0.1 9"},
	}
	for _, probe := range probes {
		res, err := guard.Execute(ctx, ExecuteRequest{
			ActionID: tc + "-" + probe.name, ActionType: "process.exec",
			Params: map[string]string{"binary": probe.binary, "args": probe.args},
		})
		assertBoundaryDenied(t, tc, probe.name, res, err)
		counters.RecordBoundaryRefusal(probe.name)
		counters.RecordBypassPass()
	}
	_ = os.Remove(filepath.Join(home, ".goalos-bypass-probe-"+tc)) // 边界失败时的残留清理

	// 对照组：代理层拒绝（参数非法——未触 OS 边界）不得计入旁路通过
	_, err3 := guard.Execute(ctx, ExecuteRequest{
		ActionID: tc + "-proxy", ActionType: "process.exec",
		Params: map[string]string{"binary": ""},
	})
	if err3 == nil {
		t.Fatalf("%s 对照组失败：空 binary 应被代理层拒绝", tc)
	}
	counters.RecordProxyRefusal("empty binary")

	// TC-RT-002 计数断言：代理拒绝与边界拒绝分离；旁路通过仅计边界拒绝
	if counters.ProxyRefusals != 1 {
		t.Fatalf("%s：ProxyRefusals 应=1，实际 %d", tc, counters.ProxyRefusals)
	}
	if counters.BoundaryRefusals != 3 {
		t.Fatalf("%s：BoundaryRefusals 应=3，实际 %d", tc, counters.BoundaryRefusals)
	}
	if counters.BypassPasses() != 3 {
		t.Fatalf("%s：BypassPasses 应=3（仅边界拒绝计入），实际 %d", tc, counters.BypassPasses())
	}

	if err := guard.Release(ctx); err != nil {
		t.Fatalf("%s：Release 失败: %v", tc, err)
	}
}

// assertBoundaryDenied 断言请求被 OS 边界拒绝（子进程级 EPERM 形态）。
// 反虚假绿（2026-08-29 实证事故——execvp 假象）：输出含 "sandbox-exec:" 前缀=
// 边界从未生效（进程未启动），一律不计为边界证据；成功执行=边界失效=CRITICAL。
func assertBoundaryDenied(t *testing.T, tc, what string, res ExecuteResult, err error) {
	t.Helper()
	if err == nil && res.ExitCode == 0 {
		t.Fatalf("%s CRITICAL：%s 旁路成功——OS 边界失效（边界防线零证据）", tc, what)
	}
	if strings.Contains(res.Output, "sandbox-exec:") {
		t.Fatalf("%s：%s 为 execvp 级失败（边界从未生效的伪证——不计数）——Output=%q", tc, what, res.Output)
	}
	if !strings.Contains(res.Output, "Operation not permitted") {
		t.Fatalf("%s：%s 非 OS 边界拒绝形态（应为子进程级 EPERM）——ExitCode=%d Output=%q err=%v",
			tc, what, res.ExitCode, res.Output, err)
	}
}
