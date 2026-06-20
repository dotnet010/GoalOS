// Package channel — Telegram Bot 参考实现。
// 使用长轮询（getUpdates）接收消息。零配置——出站 HTTPS 连接。
// 核心团队维护此参考实现。社区可基于 channel.Plugin 接口适配其他平台。
//
// 设计依据：05 架构文档 §4.3、R68。
package channel

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
)

// TelegramBot 是 Telegram Bot 的 Channel Plugin 实现。
// 使用 Bot API 长轮询接收消息。零公网 IP 要求。
type TelegramBot struct {
	token       string
	apiURL      string
	offset      int
	stopCh      chan struct{}
	httpClient  *http.Client
}

// NewTelegramBot 创建 Telegram Bot。
// token 是从 @BotFather 获取的 Bot Token。
func NewTelegramBot(token string) *TelegramBot {
	return &TelegramBot{
		token:      token,
		apiURL:     "https://api.telegram.org/bot" + token,
		stopCh:     make(chan struct{}),
		httpClient: &http.Client{Timeout: 35 * time.Second}, // 长轮询超时 30s + 5s 缓冲
	}
}

// Name 返回通道名称。
func (tb *TelegramBot) Name() string { return "telegram" }

// Start 启动长轮询循环。收到用户消息时调用 callback。
func (tb *TelegramBot) Start(callback func(Message) error) error {
	go tb.pollLoop(callback)
	return nil
}

// pollLoop 是长轮询主循环。
func (tb *TelegramBot) pollLoop(callback func(Message) error) {
	for {
		select {
		case <-tb.stopCh:
			return
		default:
		}

		updates, err := tb.getUpdates()
		if err != nil {
			time.Sleep(time.Second) // 出错退避
			continue
		}

		for _, upd := range updates {
			if upd.Message == nil || upd.Message.Text == "" {
				tb.offset = upd.UpdateID + 1
				continue
			}

			msg := Message{
				Channel:    "telegram",
				SenderID:   fmt.Sprintf("%d", upd.Message.From.ID),
				SenderName: upd.Message.From.FirstName,
				Content:    upd.Message.Text,
			}
			if err := callback(msg); err != nil {
				continue // 回调失败不停止轮询
			}
			tb.offset = upd.UpdateID + 1
		}
	}
}

// getUpdates 调用 Telegram Bot API getUpdates。
func (tb *TelegramBot) getUpdates() ([]update, error) {
	url := fmt.Sprintf("%s/getUpdates?offset=%d&timeout=30", tb.apiURL, tb.offset)
	resp, err := tb.httpClient.Get(url)
	if err != nil {
		return nil, fmt.Errorf("telegram: getUpdates 失败: %w", err)
	}
	defer resp.Body.Close()

	var result struct {
		OK     bool     `json:"ok"`
		Result []update `json:"result"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("telegram: 解析响应失败: %w", err)
	}
	return result.Result, nil
}

// Send 发送消息到指定聊天。
func (tb *TelegramBot) Send(chatID string, content MessageContent) error {
	text := content.Text
	if content.Markdown != "" {
		text = content.Markdown
	}

	body := map[string]interface{}{
		"chat_id": chatID,
		"text":    text,
	}

	// 如果有按钮→添加 inline keyboard
	if len(content.Buttons) > 0 {
		var row []map[string]string
		for _, b := range content.Buttons {
			row = append(row, map[string]string{
				"text":          b.Label,
				"callback_data": b.Action,
			})
		}
		body["reply_markup"] = map[string]interface{}{
			"inline_keyboard": [][]map[string]string{row},
		}
	}

	data, _ := json.Marshal(body)
	resp, err := tb.httpClient.Post(
		tb.apiURL+"/sendMessage",
		"application/json",
		bytes.NewReader(data),
	)
	if err != nil {
		return fmt.Errorf("telegram: sendMessage 失败: %w", err)
	}
	defer resp.Body.Close()

	// 检查 Telegram API 错误
	var result struct {
		OK          bool   `json:"ok"`
		Description string `json:"description"`
	}
	io.ReadAll(resp.Body)
	// W7: 简化处理。完整错误处理在 W8。
	_ = result
	return nil
}

// Capabilities 返回 Telegram 平台能力。
func (tb *TelegramBot) Capabilities() Capabilities {
	return Capabilities{
		Name:             "Telegram",
		SupportsMarkdown: true,
		SupportsButtons:  true,
		MaxMessageLength: 4096,
	}
}

// Stop 停止长轮询。
func (tb *TelegramBot) Stop() error {
	close(tb.stopCh)
	return nil
}

// ─── Telegram API 类型 ───

type update struct {
	UpdateID int            `json:"update_id"`
	Message  *telegramMsg   `json:"message"`
}

type telegramMsg struct {
	MessageID int           `json:"message_id"`
	From      telegramUser  `json:"from"`
	Chat      telegramChat  `json:"chat"`
	Text      string        `json:"text"`
}

type telegramUser struct {
	ID        int64  `json:"id"`
	FirstName string `json:"first_name"`
}

type telegramChat struct {
	ID int64 `json:"id"`
}
