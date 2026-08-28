// resolver_table_contract_test.go——任务 3.1 决策表全表契约测试（R-571 先红；
// 05 §X.6.4 六行按序求值+R-1601 行 1 硬地板+R-1602 I5 显式+R-1628③ typed 比较）。
// 断言来源：开发计划任务 3.1 验收标准（决策表六行全覆盖；降级边界=硬地板断言）。
package runtime

import (
	"testing"
	"time"
)

// TestRuntime_Resolver_DecisionTable 决策表六行全覆盖+按序首匹配+硬地板。
// 行 1：RRE=false ∧ 名单命中未过期 ∧ MinIsolation≤I1 → T0
// 行 2：RRE=false 但名单缺失/过期 → 按 true 重判（不静默留 T0）
// 行 3：RRE=true ∧ MinIsolation≤I3 ∧ 平台达成≥MinIsolation → T1
// 行 3b：平台达成<MinIsolation → 治理升级（NeedsGovernanceEscalation）或拒绝
// 行 4：MinIsolation=I4 ∧ 本机 I4 后端可用 → T2（v0.3.1 无后端=永不命中——R-1599）
// 行 5：MinIsolation=I4 ∧ 后端不可用 → 拒绝（T3 分支 v0.3.1=拒绝——R-1483）
// 行 6：无满足候选 → 拒绝（严禁降档）；I5=i5_not_implemented（R-1602）
func TestRuntime_Resolver_DecisionTable(t *testing.T) {
	future := time.Now().Add(time.Hour).UTC().Format(time.RFC3339)
	entry := TrustedWorkload{
		PublisherKey: "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef",
		ArtifactHash: "abababababababababababababababababababababababababababababababab",
		ExpiresAt:    future,
	}

	mk := func(platformMax IsolationLevel) *Resolver {
		return NewResolver([]TrustedWorkload{entry}).WithPlatformMaxIsolation(func() IsolationLevel { return platformMax })
	}

	// 行 1
	sel, err := mk(I2).Resolve(ResolveInput{MinIsolation: I1, WorkloadHashHex: entry.ArtifactHash})
	if err != nil || sel.Tier != TierT0 || sel.MatchedWorkload == "" {
		t.Fatalf("行 1 失败: %+v err=%v", sel, err)
	}
	// 行 1 硬地板（R-1601）：MinIsolation=I3 即使名单命中也禁止 T0
	sel, err = mk(I4).Resolve(ResolveInput{MinIsolation: I3, WorkloadHashHex: entry.ArtifactHash})
	if err != nil || sel.Tier == TierT0 {
		t.Fatalf("行 1 硬地板失败（MinIsolation>I1 禁 T0）: %+v err=%v", sel, err)
	}
	// 行 2+3：名单缺失 → 重判 → 平台达成足够 → T1
	sel, err = mk(I3).Resolve(ResolveInput{MinIsolation: I2, WorkloadHashHex: "cdcd"})
	if err != nil || sel.Tier != TierRestricted || sel.MatchedWorkload != "" {
		t.Fatalf("行 2/3 失败: %+v err=%v", sel, err)
	}
	// 行 3b：平台达成不足（平台 I2，需求 I3）→ 治理升级标记（不静默降级）
	sel, err = mk(I2).Resolve(ResolveInput{RequiresRealEnforcement: true, MinIsolation: I3})
	if err != nil {
		t.Fatalf("行 3b 应返回治理升级标记非错误: %v", err)
	}
	if !sel.NeedsGovernanceEscalation {
		t.Fatalf("行 3b 失败：平台落差未标记治理升级: %+v", sel)
	}
	// 行 4：I4 需求+后端可用 → T2（v0.3.1 永不命中——注入可用后端验证逻辑分支存在性）
	sel, err = NewResolver(nil).
		WithPlatformMaxIsolation(func() IsolationLevel { return I4 }).
		WithI4BackendAvailable(func() bool { return true }).
		Resolve(ResolveInput{RequiresRealEnforcement: true, MinIsolation: I4})
	if err != nil || sel.Tier != TierHardenedLocal {
		t.Fatalf("行 4 失败（I4 后端可用应命中 T2）: %+v err=%v", sel, err)
	}
	// 行 5：I4 需求+后端不可用 → 拒绝（T3 分支 v0.3.1=拒绝）
	_, err = NewResolver(nil).
		WithPlatformMaxIsolation(func() IsolationLevel { return I4 }).
		Resolve(ResolveInput{RequiresRealEnforcement: true, MinIsolation: I4})
	if err == nil {
		t.Fatal("行 5 失败：I4 需求+后端不可用应拒绝")
	}
	// 行 6：I5 显式拒绝
	_, err = mk(I4).Resolve(ResolveInput{RequiresRealEnforcement: true, MinIsolation: I5})
	if err == nil {
		t.Fatal("行 6 失败：I5 应显式拒绝（i5_not_implemented）")
	}
}
