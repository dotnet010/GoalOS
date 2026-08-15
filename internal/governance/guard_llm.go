// guard_llm.go — Guard LLM 前置审查实现（任务 5.12——R-1081 会议 #193 威胁模型升级）。
//
// 契约：三层验证——①静态确定性层（CommandSpec/TOML profile/参数白名单，零延迟——
// 已有机制收敛为统一入口）②Guard LLM（第二模型、跨 Provider 部署 OpenAI↔Ollama 交叉；
// 不可信输入以数据非指令呈现+显式分隔符协议；结构化 verdict=ALLOW/DENY/ESCALATE）
// ③人类终审。DENY=fail-closed；ALLOW 不自证；guard 不可用→最高审批级。
package governance

import "errors"

var ErrNotImplemented = errors.New("guard llm: not implemented (skeleton stage)")

// GuardVerdict — Guard 三层验证的结构化裁决（R-1339 词汇统一：ALLOW/DENY/ESCALATE）。
type GuardVerdict string

const (
	GuardAllow    GuardVerdict = "ALLOW"    // 允许（不自证——不等于安全）
	GuardDeny     GuardVerdict = "DENY"     // 拒绝（fail-closed）
	GuardEscalate GuardVerdict = "ESCALATE" // 升级人类终审
)

// GuardLLM — Guard LLM 前置审查（三层验证统一入口）。
type GuardLLM struct{}

// ReviewInput — Guard LLM 前置审查输入（R-1466——发现 26：governance 自己拥有的
// 具名类型，不 import sandbox——编排层转译：pluginrunner 把 sandbox.CompiledProfile
// 字段抽取构造 ReviewInput）。
type ReviewInput struct {
	Command    string   // 命令字符串
	AllowPaths []string // 文件系统白名单路径（CompiledProfile.Filesystem().AllowPaths 抽取）
	NetMode    string   // 网络模式（CompiledProfile.Network().Mode 抽取）
}

// Review — 前置审查（三层验证）。
// ①静态确定性层（零延迟）→②Guard LLM（跨 Provider 交叉）→③人类终审。
// 契约：DENY=fail-closed（拒绝执行）；guard 不可用→最高审批级（ESCALATE）。
func (g *GuardLLM) Review(input ReviewInput) (GuardVerdict, error) {
	// 骨架：三层验证实现归 5.12 完成态——静态确定性层收敛为统一入口。
	// 契约：静态确定性层零延迟（CommandSpec/TOML profile/参数白名单——已有机制）。
	// 骨架：三层验证实现归 5.12 完成态——静态确定性层收敛为统一入口。
	// R-1468（发现 29）：骨架期不返回裁决类具体枚举值（GuardEscalate 不可信）——
	// 统一走 Skeleton[GuardVerdict].Unwrap() 显式未实现 error。
	return "", ErrNotImplemented
}
