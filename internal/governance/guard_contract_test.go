// 契约测试：guard 决策点必须存在，guard 不可用→拒绝 Action（R-1338）。
//
// 断言来源: R-1338（guard 不可用→拒绝 Action(guard_unavailable)+SecurityIncident，
//   fail-closed；检查点唯一=五引擎内审批前——R-1377）+ R-1340（guard 配置节：
//   provider/model/timeout_ms=5000/budget_tokens=1024/scan_budget_bytes_per_session）
//   + R-1385（pkg/events 四常量注册——SecurityIncident 同批）。
//
// 先红状态（2026-08-14）: config.Config 无 Guard 字段（R-1340 guard 配置节未落地）
//   → 探针 A 红（本测试红的充分条件）；pkg/events 已注册 TypeSecurityIncident
//   （B-7/R-1385 已传播）→ 探针 B/C 绿。
//
// 转绿任务: 3.26（guard 配置节——R-1340）+ 3.27（guard 决策点接入五引擎判定内、
//   审批决策前——R-1377 检查点唯一；guard 不可用→ActionRejected(reject_reason=
//   guard_unavailable)+SecurityIncident(severity=WARN, module='guard')——R-1338）。
//
// 契约 MUST（R-1338/R-1340/R-1385）:
//   - MUST 1: config.Config 必须含 Guard 配置节（结构体字段——R-1340 五键载体）。
//   - MUST 2: pkg/events 必须注册 TypeSecurityIncident 常量（R-1385）。
//   - MUST 3: TypeSecurityIncident 的 wire 值必须为 "security_incident"
//     （07 注册表 X.8——guard 不可用→SecurityIncident(severity=WARN, module='guard')）。
//
// 纪律: 编译安全探针（reflect/常量引用）——禁止源码文本断言。逐条 t.Error
// 而非 FailNow——一次性报告全部缺口。

package governance_test

import (
	"reflect"
	"testing"

	"github.com/goalos/goalos/internal/config"
	"github.com/goalos/goalos/pkg/events"
)

func TestGuardUnavailable_RejectsAction(t *testing.T) {
	gaps := 0

	// 探针 A（R-1340）: config.Config 必须含 Guard 配置节。
	cfgType := reflect.TypeOf(config.Config{})
	guardField, ok := cfgType.FieldByName("Guard")
	if !ok {
		t.Error("MUST 1（R-1340）: config.Config 缺少 Guard 字段——guard 配置节（provider/model/timeout_ms/budget_tokens/scan_budget_bytes_per_session）未落地，guard 决策点无法配置")
		gaps++
	} else if guardField.Type.Kind() != reflect.Struct {
		t.Error("MUST 1（R-1340）: config.Config.Guard 不是结构体——guard 配置节必须是配置段结构")
		gaps++
	}

	// 探针 B（R-1385）: pkg/events 注册 TypeSecurityIncident 常量。
	// 常量引用即编译期注册探针——此处补运行时非空检查。
	if events.TypeSecurityIncident == "" {
		t.Error("MUST 2（R-1385）: events.TypeSecurityIncident 为空——常量注册不完整")
		gaps++
	}

	// 探针 C（R-1338/07 X.8）: wire 值必须为 "security_incident"。
	if events.TypeSecurityIncident != "security_incident" {
		t.Errorf("MUST 3（R-1338）: TypeSecurityIncident wire 值=%q，必须为 %q（07 注册表 X.8）", events.TypeSecurityIncident, "security_incident")
		gaps++
	}

	if gaps > 0 {
		t.Errorf("guard 决策点契约缺口 %d 项——guard 不可用→拒绝 Action(guard_unavailable)+SecurityIncident(module='guard') 无法落地（R-1338/R-1377）", gaps)
	}
}
