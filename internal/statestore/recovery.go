// Package statestore — 冷启动恢复逻辑。
// Daemon 启动时：加载最新 snapshot → 回放增量 events.jsonl → 恢复 Goal 状态。
// O(1) 冷启动：最多回放 99 条增量事件（N=100 快照间隔）。
//
// 设计依据：05 架构文档 §12、R151、R219。
package statestore

import (
	"encoding/json"
	"fmt"
	"log"
	"os"
	"github.com/goalos/goalos/pkg/events"
)

// RecoverResult 是恢复操作的结果。
type RecoverResult struct {
	GoalID        string
	InternalState string
	EventsReplayed int
	FromSnapshot  bool
}

// RecoverAll 扫描 events/ 目录，恢复所有 Goal 的状态。
// 对每个 Goal：加载最新 snapshot → 回放增量事件 → 返回恢复结果。
func (s *Store) RecoverAll() ([]RecoverResult, error) {
	entries, err := os.ReadDir(s.baseDir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil // 首次启动——无事件
		}
		return nil, fmt.Errorf("statestore: 扫描 events 目录失败: %w", err)
	}

	var results []RecoverResult
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		goalID := entry.Name()
		result, err := s.recoverGoal(goalID)
		if err != nil {
			log.Printf("[StateStore] 恢复 Goal %s 失败: %v", goalID, err)
			continue
		}
		results = append(results, *result)
	}
	return results, nil
}

// recoverGoal 恢复单个 Goal 的状态。
func (s *Store) recoverGoal(goalID string) (*RecoverResult, error) {
	result := &RecoverResult{GoalID: goalID}

	// 1. 尝试加载最新 snapshot
	snapshot, err := s.LoadLatestSnapshot(goalID)
	if err != nil {
		return nil, fmt.Errorf("加载 snapshot: %w", err)
	}

	if snapshot.LastAppliedSeq > 0 {
		result.FromSnapshot = true
		log.Printf("[StateStore] Goal %s: 从 snapshot-%d.json 恢复", goalID, snapshot.LastAppliedSeq)
	}

	// 2. 回放增量事件
	evts, err := s.Replay(goalID, snapshot.LastAppliedSeq)
	if err != nil {
		return nil, fmt.Errorf("回放事件: %w", err)
	}
	result.EventsReplayed = len(evts)

	// 3. 如有事件，重建最终状态
	if len(evts) > 0 {
		// 取最后一条事件的 seq 和 type 更新状态
		var lastEvt events.Event
		json.Unmarshal(evts[len(evts)-1], &lastEvt)
		snapshot.LastAppliedSeq = lastEvt.Seq

		// 从最后一条事件推断 Goal 状态
		snapshot.InternalState = inferStateFromEvent(lastEvt.Type)
	}

	result.InternalState = snapshot.InternalState
	return result, nil
}

// inferStateFromEvent 从事件类型推断 Goal 内部状态。
func inferStateFromEvent(evtType string) string {
	switch evtType {
	case events.TypeGoalCreated:
		return "draft"
	case events.TypeMissionGenerated:
		return "planned"
	case events.TypeUserConfirmed:
		return "running"
	case events.TypeGoalPaused:
		return "paused"
	case events.TypeGoalCompleted:
		return "completed"
	case events.TypeActionFailed:
		return "recovering"
	default:
		return "running"
	}
}

// SnapshotInterval 是快照写入间隔（事件数）。
const SnapshotInterval = 100
