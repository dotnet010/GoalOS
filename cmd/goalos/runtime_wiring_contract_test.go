// runtime_wiring_contract_test.go——Runtime 边界组合根契约测试（任务 5.5 前置——R-1640②）。
// 标注=实现同步补强（非先红——诚实标注纪律）。12 清单 G 节登记。
package main

import (
	"context"
	"testing"

	"github.com/goalos/goalos/internal/config"
	"github.com/goalos/goalos/internal/eventbus"
	"github.com/goalos/goalos/internal/governance"
	goalosruntime "github.com/goalos/goalos/internal/runtime"
)

// TestDaemon_RuntimeWiring_BoundaryUp 组合根接线断言：
// ①三件套全部非空（registry/resolver/verifier）
// ②darwin 上受限档 Provider 经注册表可取回+租约可建可释（RegisterChecked 骨架探测过）
// ③解析器生产路径可解析（参考输入 RRE=true/I2→T1 受限档——平台达成实证）
// ④验证层真实拒绝（伪 token=signature_invalid 封闭枚举——非空返回式绿）。
func TestDaemon_RuntimeWiring_BoundaryUp(t *testing.T) {
	bus := eventbus.New()
	cfg := &config.Config{}
	gov := governance.New(bus, nil)

	rb := runtimeWiring(bus, t.TempDir(), cfg, gov, []byte("test-secret-32-bytes-padding!!!!!"))

	// ①三件套
	if rb == nil || rb.registry == nil || rb.resolver == nil || rb.verifier == nil {
		t.Fatal("组合根三件套必须全部非空（registry/resolver/verifier）")
	}

	// ②darwin：Provider 注册可取回+租约生命周期（其他平台注册表空=5.1/5.2 收敛前合法态，跳过）
	if goalosruntime.DetectPlatformIsolation() >= goalosruntime.I2 {
		p, err := rb.registry.AcquireProviderForTier("T1")
		if err != nil {
			t.Fatalf("受限档 Provider 未注册: %v", err)
		}
		h, err := p.Acquire(context.Background(), goalosruntime.LeaseRequest{GoalID: "wiring-test"})
		if err != nil {
			t.Fatalf("Provider 租约失败: %v", err)
		}
		if err := h.Release(context.Background()); err != nil {
			t.Fatalf("Release 失败: %v", err)
		}
	}

	// ③解析器参考解析
	if goalosruntime.DetectPlatformIsolation() >= goalosruntime.I2 {
		sel, err := rb.resolver.Resolve(goalosruntime.ResolveInput{
			RequiresRealEnforcement: true, MinIsolation: goalosruntime.I2,
		})
		if err != nil || sel.Tier != goalosruntime.TierRestricted {
			t.Fatalf("参考解析应=T1 受限档，实际: %v err=%v", sel.Tier, err)
		}
	}

	// ④验证层真实拒绝（伪 token——签名非法；封闭枚举断言非「无错即绿」）
	_, err := rb.verifier.Verify("garbage.token.value")
	if !goalosruntime.IsRejectReason(err, goalosruntime.RejectSignatureInvalid) {
		t.Fatalf("伪 token 应=signature_invalid 拒绝，实际: %v", err)
	}
}
