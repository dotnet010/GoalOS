// drift.go — 目标偏离检测实现（任务 5.13——R-1082 会议 #193 威胁模型升级）。
//
// 契约（05 §3.18 权威）：两层检测——①确定性层零误报硬契约（工具调用集 vs 目标声明集
// 偏离+循环检测+预算异常+金丝雀信号复用 R-1080 联动）②语义层统计遥测（ESN+CUSUM
// 模式检测，~200μs/步，无需第二 LLM）。信号→暂停+审批升级+通知+人类裁决（不自动阻断）；
// 闭环=drift 确认→statefs rollback（R-1366）→重跑。
package drift

import "github.com/goalos/goalos/internal/skeleton"

// DriftSignal — 漂移检测信号（两层检测输出）。
type DriftSignal struct {
	Layer    string // "deterministic"|"semantic"（确定性层/语义层）
	Reason   string // 偏离原因
	Severity string // "WARN"|"ESCALATE"（信号只升级不裁决——05 §3.18）
}

// GoalDeclarationSet — MissionGraph 目标声明集（05 §3.18 附录 schema——任务 5.14）。
// 契约：字段名区分非裸参数顺序（R-1467——发现 27：declaredCaps/invokedCaps 相邻同类型
// 参数类型系统不阻止顺序写反——schema 结构体承载消解）。
type GoalDeclarationSet struct {
	GoalID               string   // Goal ID
	DeclaredCapabilities []string // 声明的能力集（MissionGraph 节点声明的能力族）
	DeclaredTargets      []string // 声明的目标集（MissionGraph 节点声明的目标域）
}

// ToolCallSet — 工具调用集（05 §3.18 附录 schema——任务 5.14）。
type ToolCallSet struct {
	ActionID            string   // Action ID
	InvokedCapabilities []string // 实际调用的能力集（执行期工具调用流）
	InvokedTargets      []string // 实际调用的目标集（执行期工具调用流）
}

// Detector — 目标偏离检测器（两层检测统一入口）。
type Detector struct{}

// Check — 漂移检测（两层检测）。
// ①确定性层（零误报硬契约——工具调用集 vs 目标声明集偏离+循环检测+预算异常+金丝雀信号复用）
// ②语义层（ESN+CUSUM 统计遥测模式检测）。
// 契约：确定性层触发=真偏离（误报=契约测试 FAIL）；语义层=71% @5%FP 基线只升级不裁决。
func (d *Detector) Check(declared GoalDeclarationSet, invoked ToolCallSet) (skeleton.Skeleton[*DriftSignal], error) {
	// 骨架：两层检测实现归 5.13 完成态——确定性层零误报硬契约+语义层统计遥测。
	// R-1468（发现 28）+R-1473（发现 35 先例核实）：骨架期=完整 Skeleton[*DriftSignal] 包装类型
	//（携带方向标注+跟踪引用——非裸 sentinel 形态）。
	return skeleton.NotImplemented[*DriftSignal](skeleton.FailClosed, "R-1468 §drift"), nil
}
