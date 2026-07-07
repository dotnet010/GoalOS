// Package test — v0.2.0 Week 4 A-P2 contract_test + Meyer 15 闸口
package test

import (
	"testing"
	"time"

	"github.com/goalos/goalos/internal/scheduler"
)

// ─── A22: GoalCompleted 防重复 ────────────────────────────────

func TestA22_GoalCompleted_NoDuplicate(t *testing.T) {
	t.Run("GoalFailed后不触发GoalCompleted", func(t *testing.T) {
		gs := scheduler.NewGoalState("goal-1")
		gs.State = "Failed"
		if gs.CanTransitionTo("Completed") {
			t.Error("Failed 状态不应允许→Completed 转换")
		}
	})

	t.Run("Running状态允许Completed", func(t *testing.T) {
		gs := scheduler.NewGoalState("goal-2")
		gs.State = "Running"
		if !gs.CanTransitionTo("Completed") {
			t.Error("Running 状态应允许→Completed 转换")
		}
	})

	t.Run("已Completed不允许再次Completed", func(t *testing.T) {
		gs := scheduler.NewGoalState("goal-3")
		gs.State = "Completed"
		if gs.CanTransitionTo("Completed") {
			t.Error("Completed 终态不应允许再次 Completed")
		}
	})
}

// ─── A23: 状态恢复——不只是最后事件 ──────────────────────────

func TestA23_StateRecovery(t *testing.T) {
	t.Run("从多个事件重放恢复完整状态", func(t *testing.T) {
		store := scheduler.NewStateRecovery()
		store.Append("GoalCreated")
		store.Append("ActionScheduled")
		store.Append("ActionCompleted")
		store.Append("GoalCompleted")
		if store.LastState() != "GoalCompleted" {
			t.Errorf("LastState=%v, want GoalCompleted", store.LastState())
		}
	})

	t.Run("中途崩溃_恢复时不丢失前期事件", func(t *testing.T) {
		store := scheduler.NewStateRecovery()
		store.Append("GoalCreated")
		store.Append("ActionScheduled")
		store.Append("ActionCompleted")
		// 崩溃——ActionCompleted 之后没有 GoalCompleted
		state := store.Recover()
		if state != "Running" {
			t.Errorf("Recover=%v, want Running（ActionCompleted→Goal仍在Running，等待后续Action或GoalCompleted）", state)
		}
	})
}

// ─── A24: 注释与代码一致 ──────────────────────────────────────

func TestA24_CommentConsistency(t *testing.T) {
	t.Run("超时常量与文档一致", func(t *testing.T) {
		if scheduler.DefaultWaitTimeout != 30*time.Second {
			t.Errorf("DefaultWaitTimeout=%v, 文档说 30s", scheduler.DefaultWaitTimeout)
		}
	})
}

// ─── A25: 经验文件时间戳 ──────────────────────────────────────

func TestA25_ExperienceTimestamp(t *testing.T) {
	t.Run("CreatedAt不是零值", func(t *testing.T) {
		e := scheduler.NewExperience("test-goal", "lesson")
		if e.CreatedAt.IsZero() {
			t.Error("CreatedAt 不应是零值 0001-01-01——A25 修复")
		}
	})

	t.Run("CreatedAt接近当前时间", func(t *testing.T) {
		e := scheduler.NewExperience("test-goal", "decision")
		diff := time.Since(e.CreatedAt)
		if diff > time.Minute {
			t.Errorf("CreatedAt 距当前 %v, 应接近 now", diff)
		}
	})
}

// ─── A26: Pattern 异步提取 ────────────────────────────────────

func TestA26_PatternAsync(t *testing.T) {
	t.Run("Pattern提取不阻塞主流程", func(t *testing.T) {
		extractor := scheduler.NewPatternExtractor()
		done := make(chan bool, 1)
		go func() {
			extractor.Extract([]string{"goal-1", "goal-2", "goal-3"})
			done <- true
		}()
		select {
		case <-done:
			// 正常完成
		case <-time.After(100 * time.Millisecond):
			t.Error("Pattern 提取不应阻塞——应异步执行")
		}
	})

	t.Run("同领域Goal不足3个_不提取", func(t *testing.T) {
		extractor := scheduler.NewPatternExtractor()
		if extractor.ShouldExtract([]string{"goal-1", "goal-2"}) {
			t.Error("同领域 <3 个 Goal 不应触发 Pattern 提取")
		}
	})

	t.Run("同领域Goal>=3个_触发提取", func(t *testing.T) {
		extractor := scheduler.NewPatternExtractor()
		if !extractor.ShouldExtract([]string{"goal-1", "goal-2", "goal-3"}) {
			t.Error("同领域 >=3 个 Goal 应触发 Pattern 提取")
		}
	})
}
