// Package persona 实现 GoalOS Persona——消息渲染层。
// Core 产中性事件→Persona 渲染为用户可读文本。
// 内置 3 个 Persona：concise（默认）、warm、minimal。
// 用户可自定义 persona.md。
//
// 设计依据：05 架构文档 §4.3.1、R79-R87。
package persona

// Persona 定义系统的"声音"——消息渲染风格。
type Persona struct {
	Name        string // "concise"|"warm"|"minimal"
	Description string // 人类可读描述
	// 词库
	AckWord  string // 确认词。如 "收到"
	DoneWord string // 完成词。如 "已完成"
	WarnWord string // 警告词。如 "⚠"
	// 渲染函数
	Render func(eventType string, payload map[string]interface{}) string
}

// Builtin 返回内置 Persona 列表。
func Builtin() []Persona {
	return []Persona{Concise, Warm, Minimal}
}

// Concise 是默认 Persona——简洁、直接、无 emoji。
var Concise = Persona{
	Name:        "concise",
	Description: "简洁直接。无 emoji。工程师风格。",
	AckWord:     "收到",
	DoneWord:    "已完成",
	WarnWord:    "⚠",
	Render: func(evtType string, payload map[string]interface{}) string {
		switch evtType {
		case "GoalCreated":
			title, _ := payload["title"].(string)
			return "目标已创建: " + title
		case "GoalCompleted":
			return "已完成。"
		case "GoalPaused":
			return "已暂停。"
		case "ActionPendingApproval":
			desc, _ := payload["action_description"].(string)
			risk, _ := payload["risk_level"].(string)
			return "高风险操作 (" + risk + "): " + desc + "\n选项: [批准] [拒绝]"
		case "ActionApproved":
			return "已批准。"
		case "ActionRejected":
			return "已拒绝。"
		default:
			return ""
		}
	},
}

// Warm 是温暖 Persona——轻微礼貌用语。适度 Markdown。
var Warm = Persona{
	Name:        "warm",
	Description: "温和友好。轻微礼貌用语。",
	AckWord:     "好的",
	DoneWord:    "搞定了",
	WarnWord:    "需要注意",
	Render: func(evtType string, payload map[string]interface{}) string {
		switch evtType {
		case "GoalCreated":
			title, _ := payload["title"].(string)
			return "好的，已创建目标：「" + title + "」。正在分析..."
		case "GoalCompleted":
			return "搞定了！目标已全部完成。"
		case "GoalPaused":
			return "已暂停。随时可以恢复。"
		case "ActionPendingApproval":
			desc, _ := payload["action_description"].(string)
			risk, _ := payload["risk_level"].(string)
			return "需要你的决定：检测到高风险操作（" + risk + "）\n" + desc + "\n[批准] [拒绝]"
		default:
			return ""
		}
	},
}

// Minimal 是极简 Persona——纯文本。适合 CLI/脚本。
var Minimal = Persona{
	Name:        "minimal",
	Description: "极简。纯文本。最小输出。适合脚本。",
	AckWord:     "",
	DoneWord:    "Done.",
	WarnWord:    "WARN:",
	Render: func(evtType string, payload map[string]interface{}) string {
		switch evtType {
		case "GoalCreated":
			title, _ := payload["title"].(string)
			return title
		case "GoalCompleted":
			return "Done."
		case "ActionPendingApproval":
			desc, _ := payload["action_description"].(string)
			return "WARN: " + desc + " [approve/reject]"
		default:
			return ""
		}
	},
}

// Get 按名称查找 Persona。未找到返回 Concise。
func Get(name string) Persona {
	for _, p := range Builtin() {
		if p.Name == name {
			return p
		}
	}
	return Concise
}
