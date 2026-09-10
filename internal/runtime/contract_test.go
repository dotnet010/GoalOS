// 契约测试——v0.3.1 W1 Runtime 核心层 I（R-571 测试先行；断言来源=12 清单 F 节五行）：
// TC-RT-020 契约重放 / TC-RT-021 契约时效 / TC-RT-080 ProfileDigest 不一致拒绝 /
// TC-RT-010 T0 边界路由（含 matched_workload 断言——R-1589）/ TC-RT-002 旁路防误计。
// 规格权威：05 §X.6.3（ExecutionContract 字段表+凭据时序四句）/05 §X.6.4（决策表）/
// 06 §1.3（签发决策表）/07 §4.14（Runtime 族事件）。
package runtime

import (
	"testing"
	"time"

	"github.com/goalos/goalos/internal/governance"
)

// ─── 测试夹具：签发一份合法 v2 契约 ───

func issueTestContract(t *testing.T, secret []byte, mutate func(*governance.TokenClaims)) string {
	t.Helper()
	claims := governance.TokenClaims{
		GoalID:                  "goal_test",
		ActionID:                "act_001",
		Capabilities:            []string{"fs.write"},
		IssuedAt:                time.Now().Unix(),
		ExpiresAt:               time.Now().Add(5 * time.Minute).Unix(),
		Subject:                 "wl_test",
		ProfileDigest:           "aa55aa55aa55aa55aa55aa55aa55aa55aa55aa55aa55aa55aa55aa55aa55aa55",
		SessionID:               "sess_001",
		RequiresRealEnforcement: true,
		MinIsolation:            "I2",
		Nonce:                   "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef",
		IssuerKeyID:             "kid-1",
		PolicyRevision:          "rev-1",
	}
	if mutate != nil {
		mutate(&claims)
	}
	tok, err := governance.IssueToken(claims, secret)
	if err != nil {
		t.Fatalf("签发测试契约失败: %v", err)
	}
	return tok
}

// TC-RT-020（12 清单 F 节——TestRuntime_Contract_ReplayRejected）：
// 契约重放——同一 Nonce 无法激活第二会话；同会话多次租约不重复消费 Nonce
// （凭据时序四句 R-1510：Nonce 消费点=会话建立时一次）。
func TestRuntime_Contract_ReplayRejected(t *testing.T) {
	secret := make([]byte, 32)
	verifier := NewContractVerifier(secret, NewNonceRegistry())

	tok := issueTestContract(t, secret, nil)
	if _, err := verifier.VerifyAndConsume(tok); err != nil {
		t.Fatalf("首次验证+Nonce 消费应成功: %v", err)
	}
	// 同一 Nonce 第二次激活=重放——必须拒绝（ContractRejected reject_reason=nonce_replayed）
	if _, err := verifier.VerifyAndConsume(tok); err == nil {
		t.Fatal("同一 Nonce 第二次激活未拒绝——重放防线失效")
	} else if !IsRejectReason(err, "nonce_replayed") {
		t.Fatalf("拒绝原因应为 nonce_replayed，实际: %v", err)
	}
	// 同会话后续 Acquire 用会话凭据，不重复消费 Nonce（凭据时序句(3)）：
	// 重复 Verify（不消费）不得误报重放
	if _, err := verifier.Verify(tok); err != nil {
		t.Fatalf("同会话 Verify（不消费 Nonce）应通过: %v", err)
	}
}

// TC-RT-021（12 清单 F 节——TestRuntime_Contract_Expired）：
// 契约时效——过期契约拒绝（ContractRejected 事件留痕）。
func TestRuntime_Contract_Expired(t *testing.T) {
	secret := make([]byte, 32)
	verifier := NewContractVerifier(secret, NewNonceRegistry())

	tok := issueTestContract(t, secret, func(c *governance.TokenClaims) {
		c.ExpiresAt = time.Now().Add(-time.Minute).Unix() // 已过期
	})
	if _, err := verifier.VerifyAndConsume(tok); err == nil {
		t.Fatal("过期契约未拒绝")
	} else if !IsRejectReason(err, "expired") {
		t.Fatalf("拒绝原因应为 expired，实际: %v", err)
	}
}

// TC-RT-080（12 清单 F 节——TestRuntime_ProfileDigest_Mismatch）：
// ProfileDigest 不一致拒绝——租约获取前重算摘要，不一致默认拒绝+触发重新评估+事件留痕。
func TestRuntime_ProfileDigest_Mismatch(t *testing.T) {
	secret := make([]byte, 32)
	verifier := NewContractVerifier(secret, NewNonceRegistry())

	tok := issueTestContract(t, secret, nil)
	currentDigest := "ffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffff" // 与契约不符
	_, err := verifier.VerifyWithProfile(tok, currentDigest)
	if err == nil {
		t.Fatal("ProfileDigest 不一致未拒绝")
	} else if !IsRejectReason(err, "profile_digest_mismatch") {
		t.Fatalf("拒绝原因应为 profile_digest_mismatch，实际: %v", err)
	}
}

// TC-RT-010（12 清单 F 节——TestRuntime_T0Boundary_Routing；R-1589 matched_workload 断言）：
// T0 边界路由——未认证或需任意子进程的智能体被正确拒绝并路由至受限档及以上；
// 名单命中=RuntimeSelected.matched_workload=名单条目 publisher_key 短码前 8 字符；非名单路径=省略。
func TestRuntime_T0Boundary_Routing(t *testing.T) {
	list := []TrustedWorkload{{
		PublisherKey: "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef",
		ArtifactHash: "abababababababababababababababababababababababababababababababab",
		ExpiresAt:    "",
	}}
	resolver := NewResolver(list)

	// (1)名单命中+RRE=false+MinIsolation=I1 → T0，matched_workload=publisher_key 短码前 8 字符
	sel, err := resolver.Resolve(ResolveInput{
		RequiresRealEnforcement: false,
		MinIsolation:            I1,
		WorkloadHashHex:         "abababababababababababababababababababababababababababababababab",
	})
	if err != nil {
		t.Fatalf("名单命中应命中行 1: %v", err)
	}
	if sel.Tier != TierT0 {
		t.Fatalf("名单命中应为 T0，实际: %s", sel.Tier)
	}
	if sel.MatchedWorkload != "01234567" {
		t.Fatalf("matched_workload 应为 publisher_key 短码前 8 字符，实际: %q", sel.MatchedWorkload)
	}

	// (2)非名单工作负载（哈希不在名单）→ 不得命中行 1；按行 2/3 路径路由受限档及以上
	sel2, err := resolver.Resolve(ResolveInput{
		RequiresRealEnforcement: false,
		MinIsolation:            I1,
		WorkloadHashHex:         "cdcdcdcdcdcdcdcdcdcdcdcdcdcdcdcdcdcdcdcdcdcdcdcdcdcdcdcdcdcdcdcd",
	})
	if err != nil {
		t.Fatalf("非名单应按行 2/3 路径解析（重判后落受限档）: %v", err)
	}
	if sel2.Tier == TierT0 {
		t.Fatal("非名单工作负载命中 T0——T0 边界被穿透")
	}
	if sel2.MatchedWorkload != "" {
		t.Fatalf("非名单路径 matched_workload 必须省略，实际: %q", sel2.MatchedWorkload)
	}

	// (3)需任意子进程（RRE=true）→ 永不可达 T0（签发层语义，Resolver 层再确认）
	sel3, err := resolver.Resolve(ResolveInput{
		RequiresRealEnforcement: true,
		MinIsolation:            I2,
		WorkloadHashHex:         "abababababababababababababababababababababababababababababababab",
	})
	if err != nil {
		t.Fatalf("RRE=true 应按行 3 落受限档: %v", err)
	}
	if sel3.Tier == TierT0 {
		t.Fatal("RRE=true 命中 T0——决策表行 1 硬地板失效")
	}
}

// TC-RT-002（12 清单 F 节——TestRuntime_Bypass_RefusalNotCounted）：
// 旁路测试防误计——能力代理拒绝经协议发起的请求不得误计为旁路测试通过；两类防线分开计数。
func TestRuntime_Bypass_RefusalNotCounted(t *testing.T) {
	c := NewBypassCounters()

	// 能力代理层拒绝一个协议内请求（不是 OS 边界拒绝）
	c.RecordProxyRefusal("fs.read:/etc")
	// OS 边界拒绝一个直接系统调用尝试
	c.RecordBoundaryRefusal("direct-open")

	if c.ProxyRefusals != 1 || c.BoundaryRefusals != 1 {
		t.Fatalf("计数分离失败: proxy=%d boundary=%d", c.ProxyRefusals, c.BoundaryRefusals)
	}
	// 核心不变量：代理拒绝不得计入旁路通过
	if c.BypassPasses() != 0 {
		t.Fatal("代理拒绝被误计为旁路通过——TC-RT-002 防线失效")
	}
}

// ─── W2 闭合（会议 #250 会后 W1-2 验收复核——Meyer 逐 MUST 核对三缺口）───

// TestRuntime_Contract_Revoked 吊销拒绝（验证四步之(3)——TokenStore 既有机制接线）：
// 签发→撤销→验证=拒绝（reject_reason=revoked）+ContractRejected 留痕发射。
func TestRuntime_Contract_Revoked(t *testing.T) {
	secret := make([]byte, 32)
	ts := governance.NewTokenStore()
	var rejectReasons []string
	verifier := NewContractVerifier(secret, NewNonceRegistry(),
		WithRevocationStore(ts),
		WithRejectHook(func(reason string) { rejectReasons = append(rejectReasons, reason) }))

	tok := issueTestContract(t, secret, nil)
	// 撤销该契约（tokenID=goalID-actionID——R-1392 族形态）
	ts.Revoke("goal_test-act_001")
	if _, err := verifier.Verify(tok); err == nil {
		t.Fatal("已吊销契约未拒绝")
	} else if !IsRejectReason(err, "revoked") {
		t.Fatalf("拒绝原因应为 revoked，实际: %v", err)
	}
	// ContractRejected 留痕发射断言（07 §4.14——Publisher=契约验证层）
	if len(rejectReasons) != 1 || rejectReasons[0] != "revoked" {
		t.Fatalf("ContractRejected 留痕未发射或原因错误: %v", rejectReasons)
	}
}

// TestRuntime_Contract_Expired 补强：过期拒绝必须发射留痕（reject_reason=expired）。
// （补 W1 注册行的留痕断言——TC-RT-021 含「事件留痕」）
func TestRuntime_Contract_Expired_EmitsRejection(t *testing.T) {
	secret := make([]byte, 32)
	var rejectReasons []string
	verifier := NewContractVerifier(secret, NewNonceRegistry(),
		WithRejectHook(func(reason string) { rejectReasons = append(rejectReasons, reason) }))
	tok := issueTestContract(t, secret, func(c *governance.TokenClaims) {
		c.ExpiresAt = time.Now().Add(-time.Minute).Unix()
	})
	if _, err := verifier.VerifyAndConsume(tok); err == nil {
		t.Fatal("过期契约未拒绝")
	}
	if len(rejectReasons) != 1 || rejectReasons[0] != "expired" {
		t.Fatalf("留痕应为 expired×1，实际: %v", rejectReasons)
	}
}

// TestRuntime_T0Boundary_ExpiredEntryRoutesRow2 名单条目过期→行 2 重判（R-1549-4——
// 匹配失败/已过期→受限档路径，严禁静默留 T0）。
func TestRuntime_T0Boundary_ExpiredEntryRoutesRow2(t *testing.T) {
	past := time.Now().Add(-time.Hour).UTC().Format(time.RFC3339)
	list := []TrustedWorkload{{
		PublisherKey: "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef",
		ArtifactHash: "abababababababababababababababababababababababababababababababab",
		ExpiresAt:    past,
	}}
	resolver := NewResolver(list)
	sel, err := resolver.Resolve(ResolveInput{
		RequiresRealEnforcement: false,
		MinIsolation:            I1,
		WorkloadHashHex:         "abababababababababababababababababababababababababababababababab",
	})
	if err != nil {
		t.Fatalf("过期条目应走行 2→行 3 落受限档: %v", err)
	}
	if sel.Tier == TierT0 {
		t.Fatal("已过期名单条目命中 T0——过期防线失效")
	}
	if sel.MatchedWorkload != "" {
		t.Fatalf("过期路径 matched_workload 必须省略，实际: %q", sel.MatchedWorkload)
	}
}

// TestRuntime_Resolver_EmitsSelection 解析留痕（07 §4.14——每次解析=RuntimeSelected
// 留痕，R-1590-4 审计链完整；Publisher=Runtime Resolver）。
func TestRuntime_Resolver_EmitsSelection(t *testing.T) {
	list := []TrustedWorkload{{
		PublisherKey: "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef",
		ArtifactHash: "abababababababababababababababababababababababababababababababab",
	}}
	type emitted struct {
		eventType string
		sel       Selection
	}
	var got []emitted
	resolver := NewResolver(list).WithEventHook(func(et string, sel Selection, _ string) {
		got = append(got, emitted{et, sel})
	})
	if _, err := resolver.Resolve(ResolveInput{
		MinIsolation:    I1,
		WorkloadHashHex: "abababababababababababababababababababababababababababababababab",
	}); err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || got[0].eventType != "RuntimeSelected" {
		t.Fatalf("命中行 1 必须发射 RuntimeSelected，实际: %+v", got)
	}
	if got[0].sel.MatchedWorkload != "01234567" {
		t.Fatalf("事件载荷 matched_workload=短码前 8 字符，实际: %q", got[0].sel.MatchedWorkload)
	}
	// 拒绝路径发射 RuntimeSelectionRejected
	resolver2 := NewResolver(nil).WithEventHook(func(et string, _ Selection, _ string) {
		got = append(got, emitted{et, Selection{}})
	})
	_, _ = resolver2.Resolve(ResolveInput{RequiresRealEnforcement: true, MinIsolation: I5})
	if len(got) != 2 || got[1].eventType != "RuntimeSelectionRejected" {
		t.Fatalf("I5 必须发射 RuntimeSelectionRejected（i5_not_implemented），实际: %+v", got)
	}
}

// TestRuntime_T0Boundary_UnauthenticatedRoutesRow2 未认证用例（R-1628-2——会议 #251
// Beck 评审补锚）：无 hash 输入/空 hash=「认证缺失」→行 2 重判（严禁静默留 T0）。
func TestRuntime_T0Boundary_UnauthenticatedRoutesRow2(t *testing.T) {
	list := []TrustedWorkload{{
		PublisherKey: "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef",
		ArtifactHash: "abababababababababababababababababababababababababababababababab",
	}}
	resolver := NewResolver(list)
	sel, err := resolver.Resolve(ResolveInput{
		RequiresRealEnforcement: false,
		MinIsolation:            I1,
		WorkloadHashHex:         "", // 认证缺失——无运行时验证值
	})
	if err != nil {
		t.Fatalf("认证缺失应走行 2→行 3 落受限档: %v", err)
	}
	if sel.Tier == TierT0 {
		t.Fatal("认证缺失命中 T0——T0 边界被穿透（R-1549-4）")
	}
	if sel.MatchedWorkload != "" {
		t.Fatalf("认证缺失路径 matched_workload 必须省略，实际: %q", sel.MatchedWorkload)
	}
}
