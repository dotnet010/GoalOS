// StateStore 错误路径测试 — 验证损坏文件恢复/权限不足等异常场景。
// 设计依据：R238（每个模块至少 1 个错误路径测试）。
package statestore_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/goalos/goalos/internal/statestore"
	"github.com/goalos/goalos/pkg/events"
)

// TestRecoverCorruptedEvents 验证损坏的 events.jsonl 行被跳过而不影响其他事件。
// 行为：events.jsonl 包含损坏行→Replay 跳过损坏行→正常事件仍被回放。
func TestRecoverCorruptedEvents(t *testing.T) {
	dir := t.TempDir()
	store := statestore.New(dir)

	// 写入 3 个正常事件
	store.Append("goal_corrupt", events.Event{Seq: 1, Type: events.TypeGoalCreated, GoalID: "goal_corrupt", Source: "daemon"})
	store.Append("goal_corrupt", events.Event{Seq: 2, Type: events.TypeMissionGenerated, GoalID: "goal_corrupt", Source: "mission-engine"})
	store.Append("goal_corrupt", events.Event{Seq: 3, Type: events.TypeGoalCompleted, GoalID: "goal_corrupt", Source: "scheduler"})

	// 在 events.jsonl 中插入损坏行
	evtPath := filepath.Join(dir, "goal_corrupt", "events.jsonl")
	f, err := os.OpenFile(evtPath, os.O_APPEND|os.O_WRONLY, 0600)
	if err != nil {
		t.Fatalf("打开 events.jsonl 失败: %v", err)
	}
	f.Write([]byte("这不是合法的 JSON 行\n"))
	f.Write([]byte(`{"broken": true` + "\n")) // 不完整的 JSON
	f.Close()

	// Replay——应跳过损坏行
	replayed, err := store.Replay("goal_corrupt", 0)
	if err != nil {
		t.Fatalf("Replay 失败: %v", err)
	}
	if len(replayed) != 3 {
		t.Errorf("损坏行应被跳过：期望 3 个有效事件，得到 %d", len(replayed))
	}
}

// TestRecoverEmptyDir 验证空 events 目录不报错。
func TestRecoverEmptyDir(t *testing.T) {
	dir := t.TempDir()
	store := statestore.New(dir)

	results, err := store.RecoverAll()
	if err != nil {
		t.Fatalf("RecoverAll 失败: %v", err)
	}
	if len(results) != 0 {
		t.Errorf("空目录应返回 0 个恢复结果，得到 %d", len(results))
	}
}
