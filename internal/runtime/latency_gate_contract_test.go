//go:build darwin

// latency_gate_contract_test.go——TC-RT-060（任务 8.1——延迟门槛标定；R-1502 门槛口径/
// R-1507 六段可观测/R-1616 语义三分/R-1600 标定预案）。真实硬件 n≥100（本机 darwin）。
// 门槛段=Acquire→Start（R-1502：起点=Resolver 触发 Acquire，终点=Start 就绪——
// 治理签发/契约验证/Resolver 决策三段=纳入分段归因观测但不设硬闸）。
// R-1616 语义三分：darwin=工程实测值诚实标注（Product SLA=Linux/信创——darwin 不入 SLA；
// 标定通过→转硬闸=Linux/信创平台 CI 窗口；本测试=darwin 数据点产出+形态断言不设数值闸）。
// 标注=实现同步补强（非先红——标定性质）。
package runtime

import (
	"context"
	"sort"
	"testing"
	"time"

	"github.com/goalos/goalos/internal/governance"
)

// segmentSample 分段样本（六段=治理签发/契约验证/Resolver 决策/Acquire→Start 门槛段/
// 边界验证+挂载（Precheck）/执行——05 §X.6.9 R-1507 六段+R-1526 四段融合=本表权威形态）。
type segmentSample struct {
	issue, verify, resolve, gateAcquireStart, precheck, execute time.Duration
}

func percentile(samples []time.Duration, p int) time.Duration {
	if len(samples) == 0 {
		return 0
	}
	sorted := append([]time.Duration{}, samples...)
	sort.Slice(sorted, func(i, j int) bool { return sorted[i] < sorted[j] })
	return sorted[len(sorted)*p/100]
}

func p95(samples []time.Duration) time.Duration {
	if len(samples) == 0 {
		return 0
	}
	sorted := append([]time.Duration{}, samples...)
	sort.Slice(sorted, func(i, j int) bool { return sorted[i] < sorted[j] })
	return sorted[len(sorted)*95/100]
}

// TestRuntime_Latency_Gate（TC-RT-060——延迟门槛标定，真实硬件 n≥100）：
// ①六段全打点（治理签发/契约验证/Resolver/Acquire→Start 门槛段/边界验证+挂载/执行）；
// ②门槛段 P95 出数（darwin 工程实测值——R-1616 语义三分：darwin 不入 Product SLA）；
// ③形态断言（样本数/单调非负/P95≥P50——出数可信性）；
// ④标定判定注册：darwin 数据点（硬闸转换=Linux/信创平台 CI 窗口——预案 R-1600 预登记）。
func TestRuntime_Latency_Gate(t *testing.T) {
	ctx := context.Background()
	home := t.TempDir()
	secret := []byte("latency-gate-test-secret-32bytes!!")

	verifier := NewContractVerifier(secret, NewNonceRegistry())
	resolver := NewResolver(nil).WithPlatformMaxIsolation(DetectPlatformIsolation)
	provider := NewDarwinSeatbeltProvider(home+"/ws", home+"/tmp")
	if err := provider.Prepare(ctx, RuntimePlan{PlanID: "latency", Tier: TierRestricted}); err != nil {
		t.Fatalf("Provider Prepare: %v", err)
	}

	const n = 100 // 真实硬件 n≥100——R-1507
	var gateSamples []time.Duration
	var samples []segmentSample
	observed := 0
	for i := 0; i < n; i++ {
		var sm segmentSample
		// ①治理签发（签发决策表+Token 签发——真实链路）
		t0 := time.Now()
		dec := governance.ComputeIssuanceDecision(governance.IssuanceInput{
			ArbitrarySubprocess: true, RiskLevel: "R2",
		})
		claims := governance.TokenClaims{
			GoalID: "g-lat", ActionID: "a-lat", Capabilities: []string{"shell.execute"},
			IssuedAt: time.Now().Unix(), ExpiresAt: time.Now().Unix() + 300,
			Subject: "latency", SessionID: "sess-lat",
			ProfileDigest:           "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
			RequiresRealEnforcement: dec.RequiresRealEnforcement, MinIsolation: dec.MinIsolation,
			Nonce:       "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb",
			IssuerKeyID: "gen-1", PolicyRevision: "builtin-v1",
		}
		tok, err := governance.IssueToken(claims, secret)
		if err != nil {
			t.Fatal(err)
		}
		sm.issue = time.Since(t0)

		// ②契约验证（真实验签——digest 复核段在 ProfileDigest 比对；本测量=Verify 四步）
		t0 = time.Now()
		if _, err := verifier.Verify(tok); err != nil {
			t.Fatalf("契约验证失败: %v", err)
		}
		sm.verify = time.Since(t0)

		// ③Resolver 决策（真实解析——R-1012 解析期一次冻结）
		t0 = time.Now()
		if _, err := resolver.Resolve(ResolveInput{
			RequiresRealEnforcement: dec.RequiresRealEnforcement, MinIsolation: I2,
		}); err != nil {
			t.Fatalf("Resolver 决策失败: %v", err)
		}
		sm.resolve = time.Since(t0)

		// ④门槛段=Acquire→Start（R-1502 门槛口径——本段=唯一硬闸候选）
		t0 = time.Now()
		h, err := provider.Acquire(ctx, LeaseRequest{GoalID: "g-lat", ActionID: "a-lat"})
		if err != nil {
			t.Fatal(err)
		}
		if err := h.Start(ctx); err != nil {
			t.Fatal(err)
		}
		sm.gateAcquireStart = time.Since(t0)
		gateSamples = append(gateSamples, sm.gateAcquireStart)

		// ⑤边界验证+挂载（观测段——真实三探针子进程；抽样降本=每 10 次测 1 次）
		if i%10 == 0 {
			t0 = time.Now()
			if err := h.Precheck(ctx); err != nil {
				t.Fatalf("Precheck 失败: %v", err)
			}
			sm.precheck = time.Since(t0)
			// ⑥执行（观测段——/usr/bin/true 真实子进程）
			t0 = time.Now()
			if _, err := h.Execute(ctx, ExecuteRequest{ActionID: "a-lat", ActionType: "process.exec",
				Params: map[string]string{"binary": "/usr/bin/true"}}); err != nil {
				t.Fatalf("Execute 失败: %v", err)
			}
			sm.execute = time.Since(t0)
			observed++
		}
		if err := h.Release(ctx); err != nil {
			t.Fatal(err)
		}
		samples = append(samples, sm)
	}

	// ③形态断言（出数可信性）
	if len(gateSamples) != n {
		t.Fatalf("门槛段样本数应=%d，实际 %d", n, len(gateSamples))
	}
	gateP95 := p95(gateSamples)
	gateP50 := percentile(gateSamples, 50)
	if gateP95 <= 0 || gateP95 < gateP50 {
		t.Fatalf("门槛段出数失真：P95=%v P50=%v", gateP95, gateP50)
	}
	var issueSum, verifySum, resolveSum time.Duration
	for _, sm := range samples {
		issueSum += sm.issue
		verifySum += sm.verify
		resolveSum += sm.resolve
	}

	// ②出数报告（标定数据源——t.Log 输出+开发日志登记）
	t.Logf("TC-RT-060 标定出数（darwin 工程实测 n=%d）:", n)
	t.Logf("  门槛段（Acquire→Start——R-1502 口径）: P95=%v", gateP95)
	t.Logf("  观测段均值: 治理签发=%v 契约验证=%v Resolver=%v", issueSum/n, verifySum/n, resolveSum/n)
	t.Logf("  观测段（抽样 %d 次）: 边界验证+挂载/执行=见逐样本", observed)
	t.Logf("  标定判定：darwin=工程实测值（R-1616——Product SLA=Linux/信创；硬闸转换=平台 CI 窗口，预案 R-1600 预登记）")
}
