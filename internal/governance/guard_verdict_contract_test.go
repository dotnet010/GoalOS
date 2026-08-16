// 契约测试：guard verdict 三值映射（R-1339）。
//
// 断言来源: R-1339（verdict 词汇统一 safe/suspicious/escalate——safe=放行，
//   suspicious=升级人类审批（ActionPendingApproval，风险级临时提升 R-1392），
//   escalate=拒绝(guard_rejected)）+ R-1362（StateMachineViolation 同批注册）
//   + R-1385（SecurityIncident 同批注册）。
//
// 先红状态（2026-08-14）: pkg/events 已注册 TypeSecurityIncident
//   （"security_incident"）与 TypeStateMachineViolation（"state_machine_violation"）
//   （B-7/R-1385 已传播）→ 探针 B/C/D 绿；config.Config 无 Guard 字段 → 探针 A 红；
//   governance.Engine 无任何 verdict 判定公开入口 → 探针 E 红（R-1339 三值映射
//   无行为挂接点）。探针 A/E 红即本测试红。
//
// 转绿任务: 3.26（guard 配置节——R-1340）+ 3.27（guard 决策点——verdict 三值映射
//   接入五引擎判定内；safe 放行/suspicious 升级人类审批/escalate 拒绝
//   (reject_reason=guard_rejected)——R-1339）。
//
// 契约 MUST（R-1339/R-1362/R-1385）:
//   - MUST 1: config.Config 必须含 Guard 配置节（与 R-1340 同载体）。
//   - MUST 2: pkg/events 注册 TypeSecurityIncident 且 wire 值="security_incident"。
//   - MUST 3: pkg/events 注册 TypeStateMachineViolation（R-1362 同批）。
//   - MUST 4: governance.Engine 存在公开 verdict 判定入口——safe/suspicious/escalate
//     三值映射的行为载体（R-1339）。
//
// 纪律: 编译安全探针（reflect/常量引用）——禁止源码文本断言。逐条 t.Error
// 而非 FailNow——一次性报告全部缺口。

package governance_test

import (
	"reflect"
	"strings"
	"testing"

	"github.com/goalos/goalos/internal/config"
	"github.com/goalos/goalos/internal/governance"
	"github.com/goalos/goalos/pkg/events"
)

func TestGuardVerdict_ActionMapping(t *testing.T) {
	gaps := 0

	// 探针 A（R-1340）: config.Config 必须含 Guard 配置节。
	cfgType := reflect.TypeOf(config.Config{})
	guardField, ok := cfgType.FieldByName("Guard")
	if !ok {
		t.Error("MUST 1（R-1340）: config.Config 缺少 Guard 字段——guard 配置节未落地，verdict 判定无法配置")
		gaps++
	} else if guardField.Type.Kind() != reflect.Struct {
		t.Error("MUST 1（R-1340）: config.Config.Guard 不是结构体——guard 配置节必须是配置段结构")
		gaps++
	}

	// 探针 B（R-1385）: TypeSecurityIncident 常量注册且 wire 值正确。
	if events.TypeSecurityIncident != "security_incident" {
		t.Errorf("MUST 2（R-1385）: TypeSecurityIncident 注册不完整——wire 值=%q，必须为 %q", events.TypeSecurityIncident, "security_incident")
		gaps++
	}

	// 探针 C（R-1362）: TypeStateMachineViolation 常量注册（同批注册契约）。
	if events.TypeStateMachineViolation != "state_machine_violation" {
		t.Errorf("MUST 3（R-1362）: TypeStateMachineViolation 注册不完整——wire 值=%q，必须为 %q", events.TypeStateMachineViolation, "state_machine_violation")
		gaps++
	}

	// 探针 D（R-1339）: governance.Engine 必须存在公开 verdict 判定入口。
	// 反射枚举 *Engine 公开方法集——方法名含 "Verdict" 者视为三值映射入口
	// （safe=放行 / suspicious=升级 / escalate=拒绝）。
	engType := reflect.TypeOf((*governance.Engine)(nil))
	verdictEntry := false
	for i := 0; i < engType.NumMethod(); i++ {
		if strings.Contains(engType.Method(i).Name, "Verdict") {
			verdictEntry = true
			break
		}
	}
	if !verdictEntry {
		t.Error("MUST 4（R-1339）: governance.Engine 无 verdict 判定公开入口——safe=放行/suspicious=升级人类审批/escalate=拒绝(guard_rejected) 三值映射未实现")
		gaps++
	}

	if gaps > 0 {
		t.Errorf("guard verdict 契约缺口 %d 项——R-1339 三值映射无法落地（入口不存在或配置缺失）", gaps)
	}
}
