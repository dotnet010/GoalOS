// 契约测试：Guard LLM 前置审查（R-1081——会议 #193 威胁模型升级）。
//
// 断言来源: R-1081（Guard LLM 前置审查——静态确定性层→跨 Provider Guard LLM→人类终审
//   三层验证；verdict=ALLOW/DENY/ESCALATE；DENY fail-closed；ALLOW 不自证；
//   guard 不可用→最高审批级）。
//
// 先红状态: Guard 三层验证入口未实现（guard_contract_test.go 既有先红=探针）。
// 转绿任务: 3.27（C-2 表——guard 检查点）。

package governance_test

import "testing"

// TestGuardLLM_InjectionResistant — 注入样本集→DENY 或 ESCALATE（R-1081）。
// 断言：注入样本（prompt injection 模式）→verdict≠ALLOW（fail-closed）。
func TestGuardLLM_InjectionResistant(t *testing.T) {
	t.Skip("先红挂起（R-571）——转绿归任务 3.27（guard 检查点实现）")
}
