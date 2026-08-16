package sandbox

import "errors"

// classifier.go — agentbox 规则组完整枚举+R0-R5 映射（任务 3.6：R-986 W2 末前交付 08 §11.4——
// W3-4 阻塞项前置）。
//
// 契约：W0 实测=14 Forbidden 规则组/16 Escalated 规则组/~14 Allow 规则组（agentbox pin
// v0.0.0-20260409110136-e003e7574798——R-963 vendor 锁定）；R0-R5 映射全表（08 §11.4 权威）。

// RuleGroup — agentbox 规则组（W0 实测枚举）。
type RuleGroup struct {
	ID       string // 规则组 ID
	Category string // "forbidden"|"escalated"|"allow"
	Risk     string // R0-R5 风险级（映射见 RiskMap）
}

// riskMap — R0-R5 映射全表（08 §11.4 权威——R-986；R-1461 方案 1：包私有化——
// 外部拿不到 map 本身，可变性风险类型系统消失）。
// R0=无风险/R1=低风险/R2=中风险/R3=高风险/R4=严重风险/R5=致命风险。
var riskMap = map[string]string{
	"forbidden": "R5", // Forbidden 规则组=致命风险（默认拒绝）
	"escalated": "R3", // Escalated 规则组=高风险（审批）
	"allow":     "R0", // Allow 规则组=无风险（默认允许）
}

// Risk — R0-R5 风险级查询（包私有化后的唯一访问路径——R-1461 方案 1）。
// 返回 (risk, ok)：key 不存在=ok=false（查询失败显式，非零值）。
func Risk(key string) (string, bool) {
	v, ok := riskMap[key]
	return v, ok
}

// Classifier — agentbox 规则组分类器（骨架：规则组枚举+风险映射——agentbox 集成归实现任务）。
type Classifier struct {
	groups []RuleGroup // 规则组枚举（W0 实测 14F/16E/~14A）
}

// NewClassifier — 构造分类器（骨架：规则组枚举归 agentbox 集成任务）。
func NewClassifier() *Classifier {
	return &Classifier{groups: []RuleGroup{}}
}

// Classify — 命令分类（骨架：返回规则组+风险级——agentbox 集成归实现任务）。
// 契约：命令→规则组匹配→R0-R5 风险级映射。
func (c *Classifier) Classify(cmd string) (group string, risk string, err error) {
	// 骨架：agentbox 集成归实现任务（3.6 完成态）
	return "", "", ErrNotImplemented
}

// ErrNotImplemented — 骨架阶段显式未实现错误（R-1455 诚实化；R-1458 过渡期最小对齐）。
var ErrNotImplemented = errors.New("classifier: not implemented (skeleton stage)")
