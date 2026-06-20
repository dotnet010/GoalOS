package channel_test

import (
	"testing"

	"github.com/goalos/goalos/internal/channel"
)

func TestTelegramBot_Capabilities(t *testing.T) {
	// 使用空 token 创建 bot（不实际连接 Telegram API）
	bot := channel.NewTelegramBot("dummy_token")
	caps := bot.Capabilities()

	if caps.Name != "Telegram" {
		t.Errorf("expected Telegram, got %s", caps.Name)
	}
	if !caps.SupportsMarkdown {
		t.Error("Telegram should support Markdown")
	}
	if !caps.SupportsButtons {
		t.Error("Telegram should support buttons")
	}
	if caps.MaxMessageLength != 4096 {
		t.Errorf("expected 4096, got %d", caps.MaxMessageLength)
	}
}

func TestTelegramBot_Name(t *testing.T) {
	bot := channel.NewTelegramBot("dummy_token")
	if bot.Name() != "telegram" {
		t.Errorf("expected telegram, got %s", bot.Name())
	}
}

func TestMessageContent_Buttons(t *testing.T) {
	content := channel.MessageContent{
		Text: "审批请求",
		Buttons: []channel.Button{
			{Label: "批准", Action: "approve", Style: "primary"},
			{Label: "拒绝", Action: "reject", Style: "danger"},
		},
	}
	if len(content.Buttons) != 2 {
		t.Errorf("expected 2 buttons, got %d", len(content.Buttons))
	}
}
