// Package test — v0.2.0 Week 7: B7b SSE + B8 预估 + B9 failHints + B10 可配置
package test

import (
	"testing"
	"time"

	"github.com/goalos/goalos/internal/scheduler"
)

// ─── B7b: SSE 实时推送 ───────────────────────────────────────

func TestB7b_SSE_RealTimeUpdate(t *testing.T) {
	sse := scheduler.NewSSEHub()

	t.Run("客户端连接_分配ID", func(t *testing.T) {
		clientID := sse.Connect()
		if clientID == "" {
			t.Error("Connect 应返回非空 clientID")
		}
		if sse.ClientCount() != 1 {
			t.Errorf("ClientCount=%d, want 1", sse.ClientCount())
		}
	})

	t.Run("广播事件_所有客户端收到", func(t *testing.T) {
		sse2 := scheduler.NewSSEHub()
		c1 := sse2.Connect()
		c2 := sse2.Connect()
		sse2.Broadcast("GoalCreated", `{"goal_id":"g1"}`)
		// 验证两个客户端都有待发送事件
		if sse2.PendingCount(c1) == 0 {
			t.Error("c1 应收到广播事件")
		}
		if sse2.PendingCount(c2) == 0 {
			t.Error("c2 应收到广播事件")
		}
	})

	t.Run("客户端断开_清理资源", func(t *testing.T) {
		sse3 := scheduler.NewSSEHub()
		cID := sse3.Connect()
		sse3.Disconnect(cID)
		if sse3.ClientCount() != 0 {
			t.Errorf("断开后 ClientCount=%d, want 0", sse3.ClientCount())
		}
	})
}

// ─── B8: Goal 创建预估时间 ────────────────────────────────────

func TestB8_GoalCreationEstimate(t *testing.T) {
	t.Run("复杂目标_预估时间较长", func(t *testing.T) {
		est := scheduler.EstimateGoalDuration("帮我开发一个完整的CRM系统")
		if est.Duration < 30*time.Second {
			t.Errorf("复杂目标预估=%v, want ≥30s", est.Duration)
		}
		if est.NextStatus != "Aligning" {
			t.Errorf("NextStatus=%v, want Aligning", est.NextStatus)
		}
	})

	t.Run("简单目标_预估时间较短", func(t *testing.T) {
		est := scheduler.EstimateGoalDuration("查询天气")
		if est.Duration > 30*time.Second {
			t.Errorf("简单目标预估=%v, want <30s", est.Duration)
		}
	})

	t.Run("预估包含next_status提示", func(t *testing.T) {
		est := scheduler.EstimateGoalDuration("帮我写一个博客")
		if est.NextStatus == "" {
			t.Error("NextStatus 不应为空")
		}
	})
}

// ─── B9: failHints 全量 ──────────────────────────────────────

func TestB9_failHints_FullMapping(t *testing.T) {
	t.Run("10种错误映射完整", func(t *testing.T) {
		hints := scheduler.GetAllFailHints()
		if len(hints) != 10 {
			t.Errorf("failHints 数量=%d, want 10", len(hints))
		}
	})

	t.Run("每种错误有中文建议", func(t *testing.T) {
		for code, hint := range scheduler.GetAllFailHints() {
			if hint.Suggestion == "" {
				t.Errorf("%s: Suggestion 为空", code)
			}
		}
	})

	t.Run("每种错误至少有1个操作按钮", func(t *testing.T) {
		for code, hint := range scheduler.GetAllFailHints() {
			if len(hint.Buttons) == 0 {
				t.Errorf("%s: Buttons 为空", code)
			}
		}
	})

	t.Run("BUDGET_EXCEEDED有追加预算按钮", func(t *testing.T) {
		hint := scheduler.GetFailHint("BUDGET_EXCEEDED")
		found := false
		for _, b := range hint.Buttons {
			if b == "追加预算" {
				found = true
			}
		}
		if !found {
			t.Error("BUDGET_EXCEEDED 应有'追加预算'按钮")
		}
	})

	t.Run("TIMEOUT有更换模型按钮", func(t *testing.T) {
		hint := scheduler.GetFailHint("TIMEOUT")
		found := false
		for _, b := range hint.Buttons {
			if b == "更换模型" {
				found = true
			}
		}
		if !found {
			t.Error("TIMEOUT 应有'更换模型'按钮")
		}
	})
}

// ─── B10: 自动确认可配置 ──────────────────────────────────────

func TestB10_AutoConfirmConfigurable(t *testing.T) {
	t.Run("autonomous模式_自动确认", func(t *testing.T) {
		cfg := scheduler.AutoConfirmConfig{Mode: "autonomous"}
		if !cfg.ShouldAutoConfirm() {
			t.Error("autonomous 模式应自动确认")
		}
	})

	t.Run("manual模式_不自动确认", func(t *testing.T) {
		cfg := scheduler.AutoConfirmConfig{Mode: "manual"}
		if cfg.ShouldAutoConfirm() {
			t.Error("manual 模式不应自动确认")
		}
	})

	t.Run("默认模式_不自动确认", func(t *testing.T) {
		cfg := scheduler.AutoConfirmConfig{Mode: ""}
		if cfg.ShouldAutoConfirm() {
			t.Error("默认模式不应自动确认（安全默认）")
		}
	})
}
