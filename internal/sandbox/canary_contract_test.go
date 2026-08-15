// 契约测试：金丝雀三层机制（R-1080/R-1186——会议 #193/#198）。
//
// 断言来源: R-1080（金丝雀三层——假凭据本地解析陷阱避开 TruffleHog 静态绕过）；
//   R-1186（§19.2a 金丝雀三层检测定稿——假凭据/workspace 外哨兵/敏感文件陷阱+
//   CanaryTriggered 触发+季度轮换）。
//
// 先红状态: 金丝雀扫描入口未实现（canary_budget_contract_test.go 探针 D 红=既有）。
// 转绿任务: 3.26/3.27（C-2 表——guard 配置节+金丝雀扫描入口）。

package sandbox_test

import "testing"

// TestCanary_TouchTriggersAudit — 触碰 workspace 外金丝雀触发审计（R-1080/R-1186）。
// 断言：金丝雀文件被读/写→CanaryTriggered 事件发布+HumanInterventionRequested。
func TestCanary_TouchTriggersAudit(t *testing.T) {
	t.Skip("先红挂起（R-571）——转绿归任务 3.26/3.27（金丝雀扫描入口）")
}
