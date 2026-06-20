package statestore_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/goalos/goalos/internal/statestore"
	"github.com/goalos/goalos/pkg/events"
)

func tempDir(t *testing.T) string {
	t.Helper()
	dir, err := os.MkdirTemp("", "goalos-test-*")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.RemoveAll(dir) })
	return dir
}

// TestAppendAndReplay 验证事件追加和回放。
func TestAppendAndReplay(t *testing.T) {
	dir := tempDir(t)
	store := statestore.New(dir)

	evts := []events.Event{
		{Seq: 1, Type: events.TypeGoalCreated, GoalID: "goal_001"},
		{Seq: 2, Type: events.TypeMissionGenerated, GoalID: "goal_001"},
		{Seq: 3, Type: events.TypeGoalCompleted, GoalID: "goal_001"},
	}

	for _, evt := range evts {
		if err := store.Append("goal_001", evt); err != nil {
			t.Fatalf("Append failed: %v", err)
		}
	}

	replayed, err := store.Replay("goal_001", 0)
	if err != nil {
		t.Fatalf("Replay failed: %v", err)
	}
	if len(replayed) != 3 {
		t.Fatalf("expected 3 events, got %d", len(replayed))
	}
	var firstEvt events.Event
	json.Unmarshal(replayed[0], &firstEvt)
	if firstEvt.Type != events.TypeGoalCreated {
		t.Errorf("event 0: expected GoalCreated, got %s", firstEvt.Type)
	}
}

// TestAppendCreatesDir 验证首次追加时自动创建目录。
func TestAppendCreatesDir(t *testing.T) {
	dir := filepath.Join(tempDir(t), "events")
	store := statestore.New(dir)

	evt := events.Event{Seq: 1, Type: events.TypeGoalCreated, GoalID: "goal_001"}
	if err := store.Append("goal_001", evt); err != nil {
		t.Fatalf("Append failed: %v", err)
	}

	// Verify directory and file exist
	goalDir := filepath.Join(dir, "goal_001")
	if _, err := os.Stat(goalDir); os.IsNotExist(err) {
		t.Fatal("goal directory was not created")
	}
	evtFile := filepath.Join(goalDir, "events.jsonl")
	if _, err := os.Stat(evtFile); os.IsNotExist(err) {
		t.Fatal("events.jsonl was not created")
	}
}

// TestReplayFromSeq 验证从指定 seq 开始回放。
func TestReplayFromSeq(t *testing.T) {
	dir := tempDir(t)
	store := statestore.New(dir)

	for i := 1; i <= 10; i++ {
		store.Append("goal_001", events.Event{Seq: i, Type: events.TypeGoalCompleted, GoalID: "goal_001"})
	}

	replayed, err := store.Replay("goal_001", 5)
	if err != nil {
		t.Fatalf("Replay failed: %v", err)
	}
	if len(replayed) != 5 {
		t.Fatalf("expected 5 events (seq 6-10), got %d", len(replayed))
	}
	var firstEvt events.Event
	json.Unmarshal(replayed[0], &firstEvt)
	if firstEvt.Seq != 6 {
		t.Errorf("first event should be seq 6, got %d", firstEvt.Seq)
	}
}

// TestLoadSaveState 验证状态快照的加载与保存。
func TestLoadSaveState(t *testing.T) {
	dir := tempDir(t)
	store := statestore.New(dir)

	state := &statestore.GoalState{
		GoalID:        "goal_001",
		InternalState: "running",
		LastAppliedSeq: 42,
	}
	if err := store.SaveState("goal_001", state); err != nil {
		t.Fatalf("SaveState failed: %v", err)
	}

	loaded, err := store.LoadState("goal_001")
	if err != nil {
		t.Fatalf("LoadState failed: %v", err)
	}
	if loaded.InternalState != "running" {
		t.Errorf("expected running, got %s", loaded.InternalState)
	}
	if loaded.LastAppliedSeq != 42 {
		t.Errorf("expected seq 42, got %d", loaded.LastAppliedSeq)
	}
}

// TestEmptyReplay 验证空 Goal 的回放不报错。
func TestEmptyReplay(t *testing.T) {
	dir := tempDir(t)
	store := statestore.New(dir)

	replayed, err := store.Replay("nonexistent", 0)
	if err != nil {
		t.Fatalf("Replay failed: %v", err)
	}
	if len(replayed) != 0 {
		t.Errorf("expected 0 events, got %d", len(replayed))
	}
}
