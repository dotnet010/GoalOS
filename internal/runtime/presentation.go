// presentation.go——CLI 诚实呈现映射层（任务 5.5；04 §14 规格实现——R-1492）。
// 词汇纪律（R-1477）：用户可见处只用产品语义词（协作/受限/强隔离/远程）——
// T0-T3/I0-I5 工程词汇禁止出现（TC-RT-100 grep 断言）。
// 实现纪律（R-1554）：映射=内嵌常量（禁止配置文件——配置即漂移面）；语言=中文（v0.3.1）；
// 决策表新增/变更行=决策表与映射表同一决议同修（05 §X.6.4 双修纪律）。
package runtime

import "fmt"

// ─── 档位→产品词映射（00 术语表对齐；04 §14.3 JSON 机器值）───

// tierPresentation 档位呈现对（产品词+JSON 机器值）。
var tierPresentation = map[ExecutionTier]struct{ label, jsonVal string }{
	TierT0:            {"协作", "cooperative"},
	TierRestricted:    {"受限", "constrained"},
	TierHardenedLocal: {"强隔离", "hardened_local"},
}

// TierLabel 产品词（用户可见——04 §14）。
func TierLabel(t ExecutionTier) string {
	if p, ok := tierPresentation[t]; ok {
		return p.label
	}
	return "受限" // fail-closed 未知档=最低呈现（不虚构高档位）
}

// TierJSONValue JSON 机器值（04 §14.3——机器消费接口）。
func TierJSONValue(t ExecutionTier) string {
	if p, ok := tierPresentation[t]; ok {
		return p.jsonVal
	}
	return "constrained"
}

// ─── 决策行→人类句子映射表（05 §X.6.4 R-1527 唯一权威——行号不直接呈现）───

var reasonSentence = map[string]string{
	"row1_trusted_workload": "该任务为名单登记工作负载（trusted_workloads 命中），按协作模式执行",
	"row2_restricted":       "该任务主体名单登记缺失或已过期，按受限保护执行",
	"row3_restricted":       "该任务需要系统级保护，本机安全机制已验证通过",
	"row4_hardened_local":   "该任务需要强隔离保护，由本机强隔离环境执行",
	// 拒绝族（行 5/6 合一句——05 §X.6.4 映射表）：
	"i4_backend_unavailable": "当前环境无法满足该任务所需保护级别，已拒绝（不降低保护级别勉强执行）",
	"i5_not_implemented":     "当前环境无法满足该任务所需保护级别，已拒绝（不降低保护级别勉强执行）",
	"no_candidate":           "当前环境无法满足该任务所需保护级别，已拒绝（不降低保护级别勉强执行）",
}

// ReasonSentence 依据人类句子（R-1527 映射表产出）。
// 行 3b（平台落差治理分支）不经本表（R-1562——走审批流+macOS 落差文案 R-1544）。
func ReasonSentence(reasonKey string) string {
	if s, ok := reasonSentence[reasonKey]; ok {
		return s
	}
	return reasonSentence["no_candidate"] // 未知键=拒绝句（不虚构依据）
}

// ReasonNeedsWarning 行 2 命中=警告样式（R-1550——名单缺失/过期=配置异常，
// 前缀「注意：」与正常决策行视觉区分——不以普通依据行伪装异常降级）。
func ReasonNeedsWarning(reasonKey string) bool { return reasonKey == "row2_restricted" }

// ─── 状态呈现块（04 §14.1/§14.3）───

// RuntimePresentation 执行保护呈现块（daemon status 数据源+CLI 渲染输入）。
type RuntimePresentation struct {
	TierLabel      string   `json:"tier_label"`      // 产品词（协作/受限/强隔离）
	Tier           string   `json:"tier"`            // JSON 机器值
	LevelLine      string   `json:"level_line"`      // 当前级别整行（含平台注记）
	Reason         string   `json:"reason"`          // 依据人类句子
	ReasonWarning  bool     `json:"reason_warning"`  // 行 2 命中=警告样式（R-1550）
	Degraded       []string `json:"degraded"`        // 降级证据（空=无降级）
	OfflineCapable bool     `json:"offline_capable"` // 离线可执行
}

// PresentRuntime 呈现计算（04 §14 规格）：
// (1)当前级别=产品词+平台注记——macOS 受限级措辞（R-1479）：「受限（纵深防御——含已弃用
// 系统组件的诚实标注）」不得暗示与 Linux 等价内核强制；其他平台=「受限（本平台原生强制隔离）」；
// (2)依据=RuntimeSelected.selection_reason 经映射表（行 2 命中=警告样式 R-1550）；
// (3)降级=ProviderDegraded 证据（空=「无」）；
// (4)强隔离档仅 I4 探测呈现（R-1542/R-1599——探测不到=不呈现=无死呈现；远程档不呈现 T3 不做）。
func PresentRuntime(sel Selection, platform string, degraded []string) RuntimePresentation {
	p := RuntimePresentation{
		TierLabel:      TierLabel(sel.Tier),
		Tier:           TierJSONValue(sel.Tier),
		Reason:         ReasonSentence(sel.Reason),
		ReasonWarning:  ReasonNeedsWarning(sel.Reason),
		Degraded:       degraded,
		OfflineCapable: true,
	}
	if p.TierLabel == "受限" && platform == "darwin" {
		p.LevelLine = "受限（纵深防御——含已弃用系统组件的诚实标注）" // R-1479 macOS 措辞
	} else if p.TierLabel == "受限" {
		p.LevelLine = "受限（本平台原生强制隔离）"
	} else {
		p.LevelLine = p.TierLabel
	}
	return p
}

// PresentPlatformGap 平台落差文案（04 §14.1 R-1544——I3+ 需求遇 I2 平台：
// 不静默降（硬地板），不假装能保护）。
func PresentPlatformGap(gap string) string {
	return fmt.Sprintf("该任务需要的保护级别超出本平台能力——可选择人工确认降级执行或在 Linux/Windows 平台运行（%s）", gap)
}
