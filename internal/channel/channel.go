// Package channel 实现 GoalOS Channel Plugin SDK。
// 交互通道扩展接口。第三方开发者实现此接口适配新消息平台。
// 核心团队提供 Telegram 参考实现。
//
// 设计依据：05 架构文档 §4.3、R65-R69。
package channel

// Plugin 是 Channel Plugin 接口。
// 第三方开发者实现此接口以适配新的消息平台。
type Plugin interface {
	// Name 返回通道名称。如 "telegram"、"discord"。
	Name() string

	// Start 启动消息接收循环。收到用户消息时调用 callback。
	Start(callback func(Message) error) error

	// Send 发送回复给用户。
	Send(recipient string, content MessageContent) error

	// Capabilities 返回该平台支持的能力。
	Capabilities() Capabilities

	// Stop 停止消息接收。清理资源。
	Stop() error
}

// Message 是收到的用户消息。
type Message struct {
	Channel    string // "telegram"|"discord"|"web"|"cli"
	SenderID   string // 平台用户 ID
	SenderName string // 平台用户名
	Content    string // 消息文本
	ReplyTo    string // 回复的消息 ID（可选）
}

// MessageContent 是发送给用户的消息内容。
type MessageContent struct {
	Text     string    // 纯文本（必填。所有平台通用）
	Markdown string    // Markdown（可选。平台支持时使用）
	Buttons  []Button  // 交互按钮（可选。Watcher 审批）
}

// Button 是交互按钮。
type Button struct {
	Label  string // 按钮文字
	Action string // "approve"|"reject"|"open_folder"|"view_detail"
	Style  string // "primary"|"danger"|"default"
}

// Capabilities 是平台能力声明。
type Capabilities struct {
	Name               string // 平台名称
	SupportsMarkdown   bool   // 是否支持 Markdown
	SupportsButtons    bool   // 是否支持交互按钮
	MaxMessageLength   int    // 单条消息最大长度
}
