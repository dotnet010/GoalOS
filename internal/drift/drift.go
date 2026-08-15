// drift.go — 目标偏离检测实现（任务 5.13——R-1082 会议 #193 威胁模型升级）。
//
// 契约（05 §3.18 权威）：两层检测——①确定性层零误报硬契约（工具调用集 vs 目标声明集
// 偏离+循环检测+预算异常+金丝雀信号复用 R-1080 联动）②语义层统计遥测（ESN+CUSUM
// 模式检测，~200μs/步，无需第二 LLM）。信号→暂停+审批升级+通知+人类裁决（不自动阻断）；
// 闭环=drift 确认→statefs rollback（R-1366）→重跑。
package drift

import "fmt"

// DriftSignal — 漂移检测信号（两层检测输出）。
type DriftSignal struct {
	Layer    string // "deterministic"|"semantic"（确定性层/语义层）
	Reason   string // 偏离原因
	Severity string // "WARN"|"ESCALATE"（信号只升级不裁决——05 §3.18）
}

// Detector — 目标偏离检测器（两层检测统一入口）。
type Detector struct{}

// Check — 漂移检测（两层检测）。
// ①确定性层（零误报硬契约——工具调用集 vs 目标声明集偏离+循环检测+预算异常+金丝雀信号复用）
// ②语义层（ESN+CUSUM 统计遥测模式检测）。
// 契约：确定性层触发=真偏离（误报=契约测试 FAIL）；语义层=71% @5%FP 基线只升级不裁决。
func (d *Detector) Check(goalID string, declaredCaps []string, invokedCaps []string) (*DriftSignal, error) {
	// 骨架：两层检测实现归 5.13 完成态——确定性层零误报硬契约+语义层统计遥测。
	return nil, fmt.Errorf("drift detector: 骨架——实现归任务 5.13 完成态（两层检测统一入口）")
}
