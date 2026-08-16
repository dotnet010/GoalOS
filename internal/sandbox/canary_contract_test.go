// 契约测试：金丝雀三层机制（R-1080/R-1186——会议 #193/#198）。
//
// 断言来源: R-1080（金丝雀三层——假凭据本地解析陷阱避开 TruffleHog 静态绕过）；
//   R-1186（§19.2a 金丝雀三层检测定稿——假凭据/workspace 外哨兵/敏感文件陷阱+
//   CanaryTriggered 触发+季度轮换）。
//
// 当前契约形态（会议 #202~#204）: 金丝雀扫描实现任务=C-PLAT-11 stub（R-1388——
//   生效依赖代理层 v0.3 实现）。本测试断言已落地载体：
//   - R-1383 夹具落点（test/fixtures/canary_encoding_fixture.yaml）完整——
//     金丝雀凭据非空+匹配样本存在（原始字节哈希匹配锚点，R-1382）；
//   - R-1340 guard 配置键 scan_budget_bytes_per_session 落地且默认值有效
//     （预算耗尽=原始哈希匹配不降级的载体，R-1382）。
//   C-PLAT-11 转绿实现时本测试断言升级为 CanaryTriggered 触发语义的完整行为。
//
// 纪律: 编译安全探针+行为断言——禁止源码文本断言；逐条 t.Error 非 FailNow。

package sandbox_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/goalos/goalos/internal/config"
	"gopkg.in/yaml.v3"
)

// canaryDoc 是 test/fixtures/canary_encoding_fixture.yaml 的结构镜像
// （R-1262 规范化链：depth=编码层数，expected=match|no_match）。
type canaryDoc struct {
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

// TestCanary_TouchTriggersAudit — 金丝雀契约载体落地（R-1080/R-1186/R-1383 前置契约）。
// 断言：夹具完整（凭据非空+匹配样本存在）+ guard 扫描预算键落地且默认值有效——
// C-PLAT-11 扫描入口转绿时以本断言为前置。
func TestCanary_TouchTriggersAudit(t *testing.T) {
	// 夹具加载（R-1383 落点）。
	fixturePath := filepath.Join("..", "..", "test", "fixtures", "canary_encoding_fixture.yaml")
	data, err := os.ReadFile(fixturePath)
	if err != nil {
		t.Fatalf("前置: 夹具加载失败（%s）: %v——R-1383 夹具落点缺失", fixturePath, err)
	}
	var fx canaryDoc
	if err := yaml.Unmarshal(data, &fx); err != nil {
		t.Fatalf("前置: 夹具 YAML 解析失败: %v", err)
	}

	// MUST 1（R-1080）: 金丝雀凭据非空——三层机制的对抗对象必须存在。
	if fx.CanaryCredential == "" {
		t.Error("MUST 1（R-1080）: 夹具 canary_credential 为空——金丝雀凭据未定义")
	}
	// MUST 2（R-1382）: 存在 expected=match 的样本——原始字节哈希匹配锚点
	// （预算耗尽后仍须命中的契约对象，R-1382）。
	hasMatch := false
	for _, s := range fx.Samples {
		if s.Expected == "match" {
			hasMatch = true
		}
		if s.Payload == "" {
			t.Errorf("MUST 2（R-1382）: 样本 %s payload 为空——匹配锚点不可缺失", s.ID)
		}
	}
	if !hasMatch {
		t.Error("MUST 2（R-1382）: 夹具缺少 expected=match 样本——原始字节哈希匹配契约的锚点缺失")
	}
	// MUST 3（R-1340/R-1382）: guard 扫描预算键落地且默认值有效——预算耗尽后
	// 原始哈希匹配不降级的契约载体（scan_budget_bytes_per_session）。
	cfg := config.Default()
	if cfg == nil || cfg.Guard.ScanBudgetBytesPerSession <= 0 {
		t.Errorf("MUST 3（R-1340）: guard 扫描预算默认值=%d，必须 > 0——预算载体未落地", cfg.Guard.ScanBudgetBytesPerSession)
	}
}
