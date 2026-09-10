//go:build darwin

// provider_t0_darwin_test.go——协作档 Provider 契约测试（任务 5.4——R-1507/R-1528；
// TC-RT-010 联动族）。标注=实现同步补强（非先红——诚实标注纪律）。
// 断言：名单认证→行 1→T0（TC-RT-010 联动）/能力代理白名单（声明集外=代理拒绝分离计数）/
// 无契约租约 fail-closed/声明集内真实执行（边界内）。12 清单 G 节登记。
package runtime

import (
	"context"
	"os"
	"strings"
	"testing"

	"github.com/goalos/goalos/internal/governance"
)

// TestRuntime_T0Provider_CapabilityProxy 协作档端到端（夹具可达——05 §X.6.3 现状注记）：
// (1)名单认证→解析行 1→T0+matched_workload 短码（R-1589 读取点）；
// (2)能力代理白名单：声明集外执行=代理拒绝（ProxyRefusals=1——不计入边界证据 TC-RT-002）；
// (3)声明集内=真实执行（边界内——process.exec 机制层）；
// (4)无契约租约=fail-closed（R-1501 契约不过边界）。
func TestRuntime_T0Provider_CapabilityProxy(t *testing.T) {
	ctx := context.Background()
	home := t.TempDir()

	// (1)名单认证→行 1→T0（TC-RT-010 联动——名单条目+命中哈希）
	pluginBin := home + "/trusted-plugin"
	if err := os.WriteFile(pluginBin, []byte("trusted plugin binary"), 0755); err != nil {
		t.Fatal(err)
	}
	hash, err := governance.WorkloadHashOf(pluginBin)
	if err != nil {
		t.Fatal(err)
	}
	trusted := []TrustedWorkload{{
		PublisherKey: strings.Repeat("ab", 32), ArtifactHash: hash,
	}}
	r := NewResolver(trusted)
	sel, err := r.Resolve(ResolveInput{RequiresRealEnforcement: false, MinIsolation: I1, WorkloadHashHex: hash})
	if err != nil || sel.Tier != TierT0 {
		t.Fatalf("(1)名单认证应=T0 行 1，实际: %v err=%v", sel.Tier, err)
	}
	if sel.MatchedWorkload != "abababab" {
		t.Fatalf("(1)matched_workload 应=publisher_key 前 8 字符（R-1589），实际 %q", sel.MatchedWorkload)
	}

	// Provider 准备
	p := NewDarwinCollaborativeProvider(home+"/ws", home+"/tmp")
	if err := p.Prepare(ctx, RuntimePlan{PlanID: "t0-test", Tier: TierT0}); err != nil {
		t.Fatal(err)
	}
	contract := &VerifiedContract{claims: governance.TokenClaims{Capabilities: []string{"fs.read"}}}

	h, err := p.Acquire(ctx, LeaseRequest{GoalID: "g-t0", ActionID: "a1", Contract: contract})
	if err != nil {
		t.Fatal(err)
	}
	if err := h.Start(ctx); err != nil {
		t.Fatal(err)
	}
	if err := h.Precheck(ctx); err != nil {
		t.Fatal(err)
	}

	// (2)声明集外=能力代理拒绝（fs.write 未声明）
	_, err = h.Execute(ctx, ExecuteRequest{
		ActionID: "a-deny", ActionType: "fs.write",
		Params: map[string]string{"binary": "/usr/bin/true"},
	})
	if err == nil || !strings.Contains(err.Error(), "能力代理拒绝") {
		t.Fatalf("(2)声明集外应=能力代理拒绝，实际: %v", err)
	}
	ch := h.(*collaborativeHandle)
	if ch.ProxyRefusals() != 1 {
		t.Fatalf("(2)ProxyRefusals 应=1，实际 %d", ch.ProxyRefusals())
	}

	// (3)声明集内（fs.read）=放行进入 OS 边界执行
	res, err := h.Execute(ctx, ExecuteRequest{
		ActionID: "a-allow", ActionType: "fs.read",
		Params: map[string]string{"binary": "/usr/bin/true"},
	})
	if err != nil || res.Status != "success" {
		t.Fatalf("(3)声明集内应真实执行成功，实际: status=%s err=%v", res.Status, err)
	}
	if ch.ProxyRefusals() != 1 {
		t.Fatalf("(3)放行不得误计代理拒绝——计数应仍为 1，实际 %d", ch.ProxyRefusals())
	}

	// (4)无契约租约=fail-closed
	if _, err := p.Acquire(ctx, LeaseRequest{GoalID: "g-t0", ActionID: "a2"}); err == nil {
		t.Fatal("(4)无契约租约必须拒绝（R-1501）")
	}

	if err := h.Release(ctx); err != nil {
		t.Fatal(err)
	}
}
