// runtime_gate_contract_test.go——Runtime 执行门契约测试（R-1640② 激活——会议 #255/#257）。
// 标注=实现同步补强（非先红——诚实标注纪律）。12 清单 G 节登记。
// 纪律对账：阻断路径全部行为断言（错误族/封闭枚举）——无「无错即绿」。
package pluginrunner

import (
	"os"
	"path/filepath"
	goruntime "runtime"
	"testing"

	"github.com/goalos/goalos/internal/eventbus"
	"github.com/goalos/goalos/internal/governance"
	goalosruntime "github.com/goalos/goalos/internal/runtime"
	"github.com/goalos/goalos/internal/sandbox"
	"github.com/goalos/goalos/pkg/events"
)

// gateTestPlugin 夹具插件（二进制=临时文件——WorkloadHashOf 真实读盘计算）。
func gateTestPlugin(t *testing.T) *DiscoveredPlugin {
	t.Helper()
	bin := filepath.Join(t.TempDir(), "fake-plugin")
	if err := os.WriteFile(bin, []byte("fake-plugin-binary-for-gate-test"), 0755); err != nil {
		t.Fatal(err)
	}
	return &DiscoveredPlugin{BinaryPath: bin}
}

// gateTestToken 合法 v2 token（真实签发链——IssueToken+digest 真实计算）。
func gateTestToken(t *testing.T, secret []byte, minIsolation string, digestOverride string) string {
	t.Helper()
	caps := []string{"fs.read"}
	digest := digestOverride
	if digest == "" {
		var err error
		digest, _, err = sandbox.BuiltinProfileDigest(minIsolation, caps, gateTestPlatform(), "builtin-v1")
		if err != nil {
			t.Fatal(err)
		}
	}
	claims := governance.TokenClaims{
		GoalID: "g-gate", ActionID: "a-gate", Capabilities: caps,
		IssuedAt: 1, ExpiresAt: 9999999999,
		Subject: "", ProfileDigest: digest, SessionID: "sess-gate",
		RequiresRealEnforcement: true, MinIsolation: minIsolation,
		Nonce: "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef",
		IssuerKeyID: "gen-1", PolicyRevision: "builtin-v1",
	}
	tok, err := governance.IssueToken(claims, secret)
	if err != nil {
		t.Fatal(err)
	}
	return tok
}

func gateTestPlatform() sandbox.PlatformID {
	switch goruntime.GOOS {
	case "darwin":
		return sandbox.PlatformDarwin
	case "windows":
		return sandbox.PlatformWindows
	default:
		return sandbox.PlatformLinux
	}
}

// TestRuntimeGate_Enforcement 执行门六断言（验证强制+复核重算+解析留痕/拒绝）。
func TestRuntimeGate_Enforcement(t *testing.T) {
	secret := []byte("gate-test-secret-32-bytes-padding!")
	bus := eventbus.New()
	nonces := goalosruntime.NewNonceRegistry()
	verifier := goalosruntime.NewContractVerifier(secret, nonces)
	resolver := goalosruntime.NewResolver(nil).
		WithPlatformMaxIsolation(goalosruntime.DetectPlatformIsolation)
	plugin := gateTestPlugin(t)
	evtWithToken := func(tok string) events.Event {
		return events.Event{GoalID: "g-gate", Payload: map[string]interface{}{"token": tok}}
	}

	// ①门未激活（nil）=旧路径放行
	plain := &Runner{}
	if err := plain.runtimeGate(evtWithToken("garbage"), plugin); err != nil {
		t.Fatalf("①门未激活应放行旧路径，实际阻断: %v", err)
	}

	r := New(bus, secret, nil)
	r.SetRuntimeGate(verifier, resolver, "builtin-v1")

	// ②激活+无 token=fail-closed 阻断（契约驱动执行——无契约不执行）
	if err := r.runtimeGate(events.Event{GoalID: "g-gate", Payload: map[string]interface{}{}}, plugin); err == nil {
		t.Fatal("②无 token 必须阻断（无契约不执行）")
	}

	// ③伪造 token=阻断（签名非法封闭枚举）
	if err := r.runtimeGate(evtWithToken("garbage.token.value"), plugin); !goalosruntime.IsRejectReason(err, goalosruntime.RejectSignatureInvalid) {
		t.Fatalf("③伪造 token 应=signature_invalid 阻断，实际: %v", err)
	}

	// ④ProfileDigest 不一致=阻断（复核独立重算——默认拒绝+重新评估）
	badDigestTok := gateTestToken(t, secret, "I2", "0000000000000000000000000000000000000000000000000000000000000000")
	if err := r.runtimeGate(evtWithToken(badDigestTok), plugin); !goalosruntime.IsRejectReason(err, goalosruntime.RejectProfileDigestMismatch) {
		t.Fatalf("④digest 篡改造应=profile_digest_mismatch 阻断，实际: %v", err)
	}

	// ⑤合法 token（digest 真实一致）=放行（darwin I2 达成——行 3 T1）
	goodTok := gateTestToken(t, secret, "I2", "")
	if goalosruntime.DetectPlatformIsolation() >= goalosruntime.I2 {
		if err := r.runtimeGate(evtWithToken(goodTok), plugin); err != nil {
			t.Fatalf("⑤合法 token 应放行，实际: %v", err)
		}
	}

	// ⑥MinIsolation=I4+平台无 I4 后端=解析拒绝阻断（行 5/6 严禁降档——05 §X.6.4）
	i4Tok := gateTestToken(t, secret, "I4", "")
	if err := r.runtimeGate(evtWithToken(i4Tok), plugin); err == nil {
		t.Fatal("⑥I4 需求+无后端=必须阻断（严禁降档）")
	}
}
