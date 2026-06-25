// Package statestore 实现 GoalOS State Store——Event Sourcing 持久化层。
//
// 核心特性：
//   - events.jsonl：append-only。Source of Truth。fsync 保证持久性
//   - state.json：可重建状态快照。原子写入（tmp + rename）
//   - 启动回放：从 snapshot + 增量 events 重建状态。幂等
//
// 设计依据：05 架构文档 §3、§10、R196、R219。
package statestore

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"

	"github.com/goalos/goalos/pkg/events"
)

// GoalState 是一个 Goal 的状态投影 + Checkpoint。
// 可从 events.jsonl 完全重建——非 Source of Truth。
type GoalState struct {
	GoalID         string   `json:"goal_id"`
	InternalState  string   `json:"internal_state"`
	LastAppliedSeq int      `json:"last_applied_seq"`
	NodeID         string   `json:"node_id,omitempty"`          // 当前执行节点 ID
	CompletedNodes []string `json:"completed_nodes,omitempty"`  // 已完成节点列表
	CurrentState   map[string]interface{} `json:"current_state,omitempty"` // 当前状态快照
	ArtifactPaths  []string `json:"artifact_paths,omitempty"`   // 产出物路径
	TokenIDs       []string `json:"token_ids,omitempty"`        // 回滚时需撤销的 Token
	// v1.1.0: Wait 期间的执行位置。
	// PipelineState 不作为独立文件存在——是 Snapshot 的字段，从 PipelinePaused 事件推导。
	// 符合 Projection over State 原则。
	PipelineState  *PipelineState `json:"pipeline_state,omitempty"`
}

// PipelineState 记录 PipelineRunner 在 Wait 期间的执行位置（v1.1.0）。
// 不作为独立文件持久化——是 Snapshot 的字段，从 PipelinePaused 事件推导。
type PipelineState struct {
	ResumePoint     string `json:"resume_point"`     // 恢复节点 ID
	ResumePrimitive string `json:"resume_primitive"` // 恢复后从哪个原语继续："decide"|"check"
	WaitReason      string `json:"wait_reason"`      // "approval"|"dependency"|"resource"
	TimeoutAt       string `json:"timeout_at"`       // ISO 8601 超时时间
	PendingActionIDs []string `json:"pending_action_ids,omitempty"` // 等待中的 Action ID 列表
}

// Store 管理事件持久化和状态投影。
// 线程安全——每个 Store 实例内部串行写入。
type Store struct {
	baseDir string      // 事件存储根目录。如 ~/.goalos/events/
	mu      sync.Mutex  // 写入串行化锁
}

// New 创建一个 Store。baseDir 是事件存储根目录。
func New(baseDir string) *Store {
	return &Store{baseDir: baseDir}
}

// Append 向 Goal 的 events.jsonl 追加一个事件。
// 调用 fsync 保证持久性。O_APPEND 保证 POSIX 原子写入。
func (s *Store) Append(goalID string, evt events.Event) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	dir := filepath.Join(s.baseDir, goalID)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("statestore: 创建目录失败: %w", err)
	}

	f, err := os.OpenFile(
		filepath.Join(dir, "events.jsonl"),
		os.O_APPEND|os.O_CREATE|os.O_WRONLY,
		0600,
	)
	if err != nil {
		return fmt.Errorf("statestore: 打开 events.jsonl 失败: %w", err)
	}
	defer f.Close()

	data, err := json.Marshal(evt)
	if err != nil {
		return fmt.Errorf("statestore: JSON 编码事件失败: %w", err)
	}
	data = append(data, '\n')

	if _, err := f.Write(data); err != nil {
		return fmt.Errorf("statestore: 写入事件失败: %w", err)
	}
	return f.Sync()
}

// Replay 从 events.jsonl 回放事件。
// fromSeq 是排除边界——只回放 seq > fromSeq 的事件。
// fromSeq=0 回放全部事件。
func (s *Store) Replay(goalID string, fromSeq int) ([]json.RawMessage, error) {
	path := filepath.Join(s.baseDir, goalID, "events.jsonl")
	f, err := os.Open(path)
	if os.IsNotExist(err) {
		return nil, nil // 尚无事件——正常
	}
	if err != nil {
		return nil, fmt.Errorf("statestore: 打开 events.jsonl 失败: %w", err)
	}
	defer f.Close()

	var events []json.RawMessage
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		// 按 seq 过滤
		var evt struct{ Seq int `json:"seq"` }
		json.Unmarshal(scanner.Bytes(), &evt)
		if evt.Seq > fromSeq {
			// 复制字节——scanner.Bytes() 是临时缓冲区
			line := make([]byte, len(scanner.Bytes()))
			copy(line, scanner.Bytes())
			events = append(events, line)
		}
	}
	return events, scanner.Err()
}

// LoadState 加载最新的状态快照 state.json。
// 如果文件不存在——返回空的 GoalState（无事件被应用）。
func (s *Store) LoadState(goalID string) (*GoalState, error) {
	path := filepath.Join(s.baseDir, goalID, "state.json")
	f, err := os.Open(path)
	if os.IsNotExist(err) {
		return &GoalState{GoalID: goalID}, nil
	}
	if err != nil {
		return nil, fmt.Errorf("statestore: 打开 state.json 失败: %w", err)
	}
	defer f.Close()

	var state GoalState
	if err := json.NewDecoder(f).Decode(&state); err != nil {
		return nil, fmt.Errorf("statestore: 解码 state.json 失败: %w", err)
	}
	return &state, nil
}

// SaveSnapshot 写入带 seq 的快照到 snapshots/ 目录（R151, R196）。
// O(1) 冷启动：启动时加载最新 snapshot + 回放增量事件（最多 99 条）。
func (s *Store) SaveSnapshot(goalID string, state *GoalState) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	snapDir := filepath.Join(s.baseDir, goalID, "snapshots")
	if err := os.MkdirAll(snapDir, 0755); err != nil {
		return fmt.Errorf("statestore: 创建 snapshots 目录失败: %w", err)
	}

	path := filepath.Join(snapDir, fmt.Sprintf("snapshot-%d.json", state.LastAppliedSeq))
	tmp := path + ".tmp"
	f, err := os.Create(tmp)
	if err != nil {
		return fmt.Errorf("statestore: 创建 snapshot 临时文件失败: %w", err)
	}
	if err := json.NewEncoder(f).Encode(state); err != nil {
		f.Close()
		return fmt.Errorf("statestore: 编码 snapshot 失败: %w", err)
	}
	f.Close()
	return os.Rename(tmp, path)
}

// LoadLatestSnapshot 加载最新的快照（seq 最大）。
func (s *Store) LoadLatestSnapshot(goalID string) (*GoalState, error) {
	snapDir := filepath.Join(s.baseDir, goalID, "snapshots")
	entries, err := os.ReadDir(snapDir)
	if err != nil {
		return &GoalState{GoalID: goalID}, nil // 无快照——正常
	}

	var latest *GoalState
	var latestSeq int
	for _, e := range entries {
		var seq int
		if _, err := fmt.Sscanf(e.Name(), "snapshot-%d.json", &seq); err != nil {
			continue
		}
		if seq > latestSeq {
			latestSeq = seq
			data, err := os.ReadFile(filepath.Join(snapDir, e.Name()))
			if err != nil {
				continue
			}
			var s GoalState
			if err := json.Unmarshal(data, &s); err == nil {
				latest = &s
			}
		}
	}
	if latest == nil {
		return &GoalState{GoalID: goalID}, nil
	}
	return latest, nil
}

// SaveState 持久化状态快照 state.json。使用原子写入（写入 .tmp → rename）。
func (s *Store) SaveState(goalID string, state *GoalState) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	dir := filepath.Join(s.baseDir, goalID)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("statestore: 创建目录失败: %w", err)
	}

	path := filepath.Join(dir, "state.json")
	tmp := path + ".tmp"
	f, err := os.Create(tmp)
	if err != nil {
		return fmt.Errorf("statestore: 创建 state 临时文件失败: %w", err)
	}
	if err := json.NewEncoder(f).Encode(state); err != nil {
		f.Close()
		return fmt.Errorf("statestore: 编码 state 失败: %w", err)
	}
	f.Close()

	// 原子 rename——崩溃安全
	return os.Rename(tmp, path)
}
