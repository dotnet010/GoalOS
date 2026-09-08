// bypass_shared_test.go——TC-RT-001 族共享探针（R-1452：SKIP=合法先红/FAIL=非法；
// R-571 测试先行；断言来源=12 清单 F 节 TC-RT-001a/b/c 行）。
// 旁路测试纪律（TC-RT-002 联动）：能力代理拒绝≠边界拒绝——分开计数（BypassCounters）。
// 文件布局（staticcheck U1000 按构建上下文判定——2026-09-06 CI 实证事故）：
// 本文件=三平台全用的矩阵+断言；dual 助手=linux||windows（bypass_dual_test.go）；
// 单态助手=darwin 独占（bypass_darwin_test.go）——符号定义面=使用面构造性对齐。
package runtime

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// runBypassProbeMatrix 探针矩阵+计数断言（runBypassProbe/runBypassProbeDual 共享段）。
func runBypassProbeMatrix(t *testing.T, tc string, guard *HandleGuard) {
	t.Helper()
	ctx := context.Background()
	counters := NewBypassCounters()

	// 探针组（Option B 语义——会议 #256；探针形态 per 平台——platformProbeSet 族表
	// 由平台文件注入：darwin=touch/cat/nc；linux=双引擎（bwrap/模式 B）；windows=WinAC 承载形态）
	home, _ := os.UserHomeDir()
	probes := platformProbeSet(tc, home)
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

// assertBoundaryDenied 断言请求被 OS 边界拒绝（R-1666——ERRNO 数字证据族）。
// 反虚假绿（2026-08-29 实证事故——execvp 假象）：输出含 "sandbox-exec:" 前缀=
// 边界从未生效（进程未启动），一律不计为边界证据；成功执行=边界失效=CRITICAL。
// 原生探针形态（2026-09-01）：PROBE-ERRNO=0=泄漏=CRITICAL；PROBE-ERRNO=<非零>
// =OS 拒绝（数字证据——本地化文本零依赖）；探针行缺席=形态违规=红。
func assertBoundaryDenied(t *testing.T, tc, what string, res ExecuteResult, err error) {
	t.Helper()
	if err == nil && res.ExitCode == 0 {
		t.Fatalf("%s CRITICAL：%s 旁路成功——OS 边界失效（边界防线零证据）", tc, what)
	}
	if strings.Contains(res.Output, "sandbox-exec:") {
		t.Fatalf("%s：%s 为 execvp 级失败（边界从未生效的伪证——不计数）——Output=%q", tc, what, res.Output)
	}
	// 原生探针 ERRNO 断言（R-1666）——文本证据族全退役（GBK/本地化打地鼠根治）
	if strings.Contains(res.Output, "PROBE-ERRNO=0") {
		t.Fatalf("%s CRITICAL：%s 探针操作成功——边界失效（ERRNO=0=泄漏）——Output=%q", tc, what, res.Output)
	}
	if !strings.Contains(res.Output, "PROBE-ERRNO=") {
		t.Fatalf("%s：%s 探针证据缺席（应含 PROBE-ERRNO=<n> 数字证据行）——ExitCode=%d Output=%q err=%v",
			tc, what, res.ExitCode, res.Output, err)
	}
}
