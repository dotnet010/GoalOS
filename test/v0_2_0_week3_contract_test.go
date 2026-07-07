// Package test — v0.2.0 Week 3 contract_test
// Beck 编写（R-571 测试先行）。
package test

import (
	"testing"
	"time"

	"github.com/goalos/goalos/internal/scheduler"
)

// ─── Typed Event 迁移验证 ────────────────────────────────────

func TestTypedEvent_GoalCreated_Struct(t *testing.T) {
	t.Run("GoalCreatedPayload有GoalID字段", func(t *testing.T) {
		p := scheduler.GoalCreatedPayload{GoalID: "goal-1", Title: "test"}
		if p.GoalID != "goal-1" {
			t.Error("GoalCreatedPayload.GoalID 应可读写")
		}
	})

	t.Run("ActionScheduledPayload有ActionType字段", func(t *testing.T) {
		p := scheduler.ActionScheduledPayload{
			ActionID: "a1", ActionType: "shell.execute", Target: "ls",
		}
		if p.ActionType != "shell.execute" {
			t.Error("ActionScheduledPayload.ActionType 应可读写")
		}
	})
}

// ─── B14: BudgetTracker ────────────────────────────────────────

func TestB14_BudgetTracker(t *testing.T) {
	bt := scheduler.NewBudgetTrackerWithBudget(1000000)

	t.Run("初始预算_未超限", func(t *testing.T) {
		if bt.IsExceededSimple(1000000) {
			t.Error("初始预算不应超限")
		}
	})

	t.Run("消耗后_累加", func(t *testing.T) {
		bt.RecordUsageSimple(5000)
		if bt.Used() != 5000 {
			t.Errorf("Used=%d, want 5000", bt.Used())
		}
	})

	t.Run("超过预算_IsExceeded返回true", func(t *testing.T) {
		bt2 := scheduler.NewBudgetTrackerWithBudget(100)
		bt2.RecordUsageSimple(200)
		if !bt2.IsExceededSimple(100) {
			t.Error("超过预算 IsExceeded 应返回 true")
		}
	})

	t.Run("追加预算_恢复未超限", func(t *testing.T) {
		bt3 := scheduler.NewBudgetTrackerWithBudget(100)
		bt3.RecordUsageSimple(200)
		bt3.AddBudgetSimple(200)
		if bt3.IsExceededSimple(300) {
			t.Error("追加预算后应恢复未超限")
		}
	})

	t.Run("UsageRatio_计算正确", func(t *testing.T) {
		bt4 := scheduler.NewBudgetTrackerWithBudget(1000)
		bt4.RecordUsageSimple(800)
		ratio := bt4.UsageRatioSimple(1000)
		if ratio < 0.79 || ratio > 0.81 {
			t.Errorf("UsageRatio=%v, want ~0.8", ratio)
		}
	})

	t.Run("Reset_计数器归零", func(t *testing.T) {
		bt5 := scheduler.NewBudgetTrackerWithBudget(1000)
		bt5.RecordUsageSimple(500)
		bt5.ResetSimple()
		if bt5.Used() != 0 {
			t.Errorf("Reset 后 Used=%d, want 0", bt5.Used())
		}
	})
}

// ─── J5: Token 续期 ───────────────────────────────────────────

func TestJ5_TokenRenewal(t *testing.T) {
	t.Run("Token签发_TTL正确", func(t *testing.T) {
		token := scheduler.NewCapToken("goal-1", "action-1", "shell.execute", 30*time.Second)
		if token.GoalID != "goal-1" {
			t.Error("Token GoalID 不正确")
		}
		if token.ActionID != "action-1" {
			t.Error("Token ActionID 不正确")
		}
		ttl := token.ExpiresAt.Sub(token.IssuedAt)
		if ttl != 60*time.Second {
			t.Errorf("TTL=%v, want 60s (30×2)", ttl)
		}
	})

	t.Run("Wait时撤销_Resume时重签", func(t *testing.T) {
		store := scheduler.NewTokenRenewalStore()
		store.Issue("g1", "a1", "web.search", 30*time.Second)
		store.Revoke("g1-a1")
		if store.IsRevoked("g1-a1") == false {
			t.Error("撤销后应标记为已撤销")
		}
		// Resume: 重新签发
		newToken := store.Issue("g1", "a1", "web.search", 30*time.Second)
		if newToken == nil {
			t.Fatal("Resume 重签应成功")
		}
		if store.IsRevoked("g1-a1") {
			t.Error("重签后应清除撤销标记")
		}
	})
}

// ─── E1: Evaluator ─────────────────────────────────────────────

func TestE1_Evaluator(t *testing.T) {
	t.Run("相同字符串_PASS", func(t *testing.T) {
		e := scheduler.NewBinaryEvaluator()
		result := e.Evaluate("same output", "same output")
		if result != scheduler.EvalPASS {
			t.Errorf("相同字符串 Evaluate=%v, want EvalPASS", result)
		}
	})

	t.Run("不同字符串_FAIL", func(t *testing.T) {
		e := scheduler.NewBinaryEvaluator()
		result := e.Evaluate("wrong", "expected")
		if result != scheduler.EvalFAIL {
			t.Errorf("不同字符串 Evaluate=%v, want EvalFAIL", result)
		}
	})
}
