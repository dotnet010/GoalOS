// 契约测试：金丝雀预算耗尽后原始字节哈希匹配必须继续命中（R-1382）。
//
// 断言来源: R-1382（金丝雀预算耗尽原始哈希匹配不降级——仅深度解码/规范化降级；
//   预算耗尽=fail-closed guard_budget_exhausted + SecurityIncident 去重）
//   + R-1341（金丝雀行为补齐——检测点=代理层出网请求+文件写入层，单点命中即升级）
//   + R-1340（guard 配置键 scan_budget_bytes_per_session——扫描预算载体）
//   + R-1383（夹具落点=test/fixtures/canary_encoding_fixture.yaml）。
//
// 先红状态（2026-08-14）: 夹具已落位（R-1383 传播）→ 探针 A/B/C 绿；金丝雀扫描
//   公开入口不存在——pkg/seccomp 仅暴露 Profile/Default/Extended/ForRiskLevel/
//   Apply（无金丝雀扫描方法），config.Config 无 Guard 扫描预算节（R-1340 未落地）
//   → 行为探针 D 红（R-1341/R-1382 先红）。
//
// 转绿任务: 3.26/3.27/7.x（计划 C-2 表——guard 配置节含 scan_budget_bytes_per_session
//   （R-1340）+ 金丝雀扫描入口：代理层+写层检测点，预算耗尽后 depth=0 原始字节哈希
//   匹配继续命中、深度解码降级——R-1382）。
//
// 契约 MUST（R-1382/R-1341）:
//   - MUST 1: 夹具完整——canary_credential 非空。
//   - MUST 2: 夹具含 depth=0 且 expected=match 样本（原始字节哈希匹配锚点——
//     预算耗尽后必须继续命中的契约对象）。
//   - MUST 3: 夹具含 expected=no_match 负样本（零误报控制组——R-1080）。
//   - MUST 4: 金丝雀扫描公开入口存在（R-1341/R-1382 行为载体——pkg/seccomp 金丝雀
//     扫描方法或 config Guard 扫描预算键，二选一即可）。
//
// 纪律: 编译安全探针（reflect/常量引用）——禁止源码文本断言。夹具结构断言逐条
//   t.Error 而非 FailNow——一次性报告全部缺口。

package sandbox_test

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/goalos/goalos/internal/config"
	"github.com/goalos/goalos/pkg/seccomp"
	"gopkg.in/yaml.v3"
)

// canaryFixture 是 test/fixtures/canary_encoding_fixture.yaml 的结构镜像
// （R-1262 规范化链：depth=编码层数，expected=match|no_match）。
type canaryFixture struct {
	Version          int    `yaml:"version"`
	CanaryCredential string `yaml:"canary_credential"`
	Samples          []struct {
		ID       string `yaml:"id"`
		Encoding string `yaml:"encoding"`
		Depth    int    `yaml:"depth"`
		Payload  string `yaml:"payload"`
		Expected string `yaml:"expected"`
	} `yaml:"samples"`
}

func TestCanary_BudgetExhausted_KeepsRawMatch(t *testing.T) {
	gaps := 0

	// 加载夹具（R-1383 落点）。
	fixturePath := filepath.Join("..", "..", "test", "fixtures", "canary_encoding_fixture.yaml")
	data, err := os.ReadFile(fixturePath)
	if err != nil {
		t.Fatalf("夹具加载失败（%s）: %v——R-1383 夹具落点缺失", fixturePath, err)
	}
	var fx canaryFixture
	if err := yaml.Unmarshal(data, &fx); err != nil {
		t.Fatalf("夹具 YAML 解析失败: %v", err)
	}

	// 探针 A（R-1382）: canary_credential 非空。
	if fx.CanaryCredential == "" {
		t.Error("MUST 1（R-1382）: 夹具 canary_credential 为空——金丝雀凭据未定义，契约无法验证")
		gaps++
	}

	// 探针 B（R-1382）: 存在 depth=0 且 expected=match 的样本——原始字节哈希匹配锚点。
	rawMatch := false
	// 探针 C（R-1382/R-1080）: 存在 expected=no_match 的负样本。
	hasNegative := false
	for _, s := range fx.Samples {
		switch s.Expected {
		case "match", "no_match":
		default:
			t.Errorf("MUST 2/3（R-1382）: 样本 %s expected=%q 非法——必须为 match|no_match", s.ID, s.Expected)
			gaps++
		}
		if s.Depth == 0 && s.Expected == "match" {
			rawMatch = true
		}
		if s.Expected == "no_match" {
			hasNegative = true
		}
	}
	if !rawMatch {
		t.Error("MUST 2（R-1382）: 夹具缺少 depth=0 且 expected=match 样本——预算耗尽后原始字节哈希匹配契约的锚点缺失")
		gaps++
	}
	if !hasNegative {
		t.Error("MUST 3（R-1080/R-1382）: 夹具缺少 expected=no_match 负样本——零误报控制组缺失")
		gaps++
	}

	// 探针 D（R-1341/R-1382）: 金丝雀扫描公开入口。
	// 载体二选一：(a) config.Config.Guard 节含扫描预算字段（scan_budget_bytes_per_session
	//   ——R-1340 键）；(b) pkg/seccomp 暴露金丝雀扫描方法。
	guardScanBudget := guardHasScanBudget(t)
	seccompCanary := seccompHasCanaryMethod(t)
	if !guardScanBudget && !seccompCanary {
		t.Error("MUST 4（R-1341/R-1382 先红）: 金丝雀扫描入口未实现——pkg/seccomp 无金丝雀扫描方法且 config.Config 无 Guard 扫描预算键（scan_budget_bytes_per_session）；预算耗尽后原始字节哈希匹配不降级契约无行为载体")
		gaps++
	}

	if gaps > 0 {
		t.Errorf("金丝雀预算契约缺口 %d 项——R-1382（预算耗尽原始哈希匹配不降级）未落地", gaps)
	}
}

// guardHasScanBudget 反射检查 config.Default() 的 Guard 节是否含扫描预算字段
// （scan_budget_bytes_per_session——R-1340 键，字段名含 "ScanBudget"）。
func guardHasScanBudget(t *testing.T) bool {
	t.Helper()
	cfg := config.Default()
	if cfg == nil {
		return false
	}
	typ := reflect.TypeOf(*cfg)
	f, ok := typ.FieldByName("Guard")
	if !ok || f.Type.Kind() != reflect.Struct {
		return false
	}
	st := f.Type
	for i := 0; i < st.NumField(); i++ {
		if strings.Contains(strings.ToLower(st.Field(i).Name), "scanbudget") {
			return true
		}
	}
	return false
}

// seccompHasCanaryMethod 反射检查 *seccomp.Profile 公开方法中是否存在金丝雀扫描
// 入口（方法名含 "Canary"）。
func seccompHasCanaryMethod(t *testing.T) bool {
	t.Helper()
	pt := reflect.TypeOf((*seccomp.Profile)(nil))
	for i := 0; i < pt.NumMethod(); i++ {
		if strings.Contains(strings.ToLower(pt.Method(i).Name), "canary") {
			return true
		}
	}
	return false
}
