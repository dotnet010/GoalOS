// Package test — v0.2.0 Phase B Week 5-6 contract_test
package test

import (
	"testing"

	"github.com/goalos/goalos/internal/scheduler"
)

// ─── B4: CheckResult 枚举验证 ─────────────────────────────────

func TestB4_CheckResult_EnumValidation(t *testing.T) {
	t.Run("PASS是合法枚举值", func(t *testing.T) {
		if !scheduler.IsValidCheckResult("PASS") {
			t.Error("PASS 应是合法 CheckResult")
		}
	})
	t.Run("WARN是合法枚举值", func(t *testing.T) {
		if !scheduler.IsValidCheckResult("WARN") {
			t.Error("WARN 应是合法 CheckResult")
		}
	})
	t.Run("BLOCK是合法枚举值", func(t *testing.T) {
		if !scheduler.IsValidCheckResult("BLOCK") {
			t.Error("BLOCK 应是合法 CheckResult")
		}
	})
	t.Run("REJECT是合法枚举值", func(t *testing.T) {
		if !scheduler.IsValidCheckResult("REJECT") {
			t.Error("REJECT 应是合法 CheckResult")
		}
	})
	t.Run("非法值_返回false", func(t *testing.T) {
		if scheduler.IsValidCheckResult("UNKNOWN") {
			t.Error("UNKNOWN 不应是合法 CheckResult")
		}
	})
	t.Run("空字符串_返回false", func(t *testing.T) {
		if scheduler.IsValidCheckResult("") {
			t.Error("空字符串不应是合法 CheckResult")
		}
	})
}

// ─── B5: LLM ToolCalls 防御 ────────────────────────────────────

func TestB5_LLM_ToolCalls_Defense(t *testing.T) {
	t.Run("空Arguments_分类为INVALID_OUTPUT", func(t *testing.T) {
		classification := scheduler.ClassifyToolCallError("", "")
		if classification != "INVALID_OUTPUT" {
			t.Errorf("空 Arguments→%v, want INVALID_OUTPUT", classification)
		}
	})
	t.Run("合法Arguments_分类为OK", func(t *testing.T) {
		classification := scheduler.ClassifyToolCallError("shell.execute", `{"cmd":"ls"}`)
		if classification != "OK" {
			t.Errorf("合法 Arguments→%v, want OK", classification)
		}
	})
	t.Run("非法JSON_分类为PARSE_ERROR", func(t *testing.T) {
		classification := scheduler.ClassifyToolCallError("shell.execute", "{invalid json}")
		if classification != "PARSE_ERROR" {
			t.Errorf("非法 JSON→%v, want PARSE_ERROR", classification)
		}
	})
}

// ─── B6: LLM API Key 防御 ─────────────────────────────────────

func TestB6_LLM_APIKey_Defense(t *testing.T) {
	t.Run("空字符串_不覆盖已有Key", func(t *testing.T) {
		result := scheduler.ResolveAPIKey("sk-existing-key", "")
		if result != "sk-existing-key" {
			t.Errorf("空字符串不应覆盖已有 Key: got %v", result)
		}
	})
	t.Run("新Key_覆盖已有Key", func(t *testing.T) {
		result := scheduler.ResolveAPIKey("sk-old", "sk-new")
		if result != "sk-new" {
			t.Errorf("新 Key 应覆盖旧 Key: got %v", result)
		}
	})
	t.Run("相同Key_不重复赋值", func(t *testing.T) {
		result := scheduler.ResolveAPIKey("sk-same", "sk-same")
		if result != "sk-same" {
			t.Errorf("相同 Key 应保持不变: got %v", result)
		}
	})
}

// ─── I8: Event layer 字段 ─────────────────────────────────────

func TestI8_EventLayer(t *testing.T) {
	t.Run("core事件_layer正确", func(t *testing.T) {
		if !scheduler.IsCoreEvent("GoalCreated") {
			t.Error("GoalCreated 应是 core 事件")
		}
	})
	t.Run("side_effect事件_layer正确", func(t *testing.T) {
		if scheduler.IsCoreEvent("MetricsSnapshot") {
			t.Error("MetricsSnapshot 应是 side_effect 事件")
		}
	})
	t.Run("未知事件_默认side_effect", func(t *testing.T) {
		if scheduler.IsCoreEvent("UnknownEvent") {
			t.Error("未知事件应默认 side_effect")
		}
	})
}

// ─── I9: IPC JSON 紧凑单行输出 ─────────────────────────────────

func TestI9_IPC_CompactJSON(t *testing.T) {
	t.Run("合法JSON单行_解析成功", func(t *testing.T) {
		valid, err := scheduler.ValidateCompactJSON(`{"type":"result","status":"success"}`)
		if err != nil {
			t.Errorf("合法单行 JSON 应解析成功: %v", err)
		}
		if !valid {
			t.Error("合法单行 JSON 应返回 true")
		}
	})
	t.Run("多行JSON_解析失败", func(t *testing.T) {
		valid, _ := scheduler.ValidateCompactJSON("{\n\"type\":\"result\"\n}")
		if valid {
			t.Error("多行 JSON 应返回 false（要求紧凑单行）")
		}
	})
	t.Run("非JSON行_忽略", func(t *testing.T) {
		valid, err := scheduler.ValidateCompactJSON("this is not json")
		if err == nil {
			t.Error("非 JSON 行应返回 error")
		}
		if valid {
			t.Error("非 JSON 行应返回 false")
		}
	})
}
