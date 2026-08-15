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
	"bytes"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"hash/crc32"
	"os"
	"path/filepath"
	"sync"

	"github.com/goalos/goalos/pkg/events"
)

// GoalState 是一个 Goal 的状态投影 + Checkpoint（v0.1.0 类型安全重写 R-362）。
// 可从 events.jsonl 完全重建——非 Source of Truth。符合 Projection over State 原则。
type GoalState struct {
	GoalID            string   `json:"goal_id"`
	InternalState     string   `json:"internal_state"` // Draft|Planned|Running|Paused|Failed|Completed
	LastAppliedSeq    int      `json:"last_applied_seq"`
	NodeID            string   `json:"node_id,omitempty"`
	NodeIDs           []string `json:"node_ids,omitempty"`      // R-839: 多节点支持
	CompletedNodes    []string `json:"completed_nodes,omitempty"`
	ArtifactPaths     []string `json:"artifact_paths,omitempty"`
	TokenIDs          []string `json:"token_ids,omitempty"`
	ApprovalPending   bool     `json:"approval_pending,omitempty"`   // v0.1.0: 是否有待审批 Action
	DataSharingApproved bool   `json:"data_sharing_approved,omitempty"` // v0.1.0: 数据外发是否已确认
	ActiveActions     int      `json:"active_actions,omitempty"`     // 当前并发 Action 数
	PipelineState     *PipelineState `json:"pipeline_state,omitempty"`
}

// PipelineState 记录 PipelineRunner 在 Wait 期间的执行位置（v0.1.0）。
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
//
// 存储完整性三层防线（R-1037/R-1040/R-1041——中文序数命名 R-1114）：
//   防线一=CRC32 条目级（每条目 CRC32 校验和）
//   防线二=seq 单调性（全局单调递增序号）
//   防线三=hash chain（每条目 prev_hash=上一条目 SHA-256，genesis=全零 32B）
type Store struct {
	baseDir     string     // ~/.goalos/events/
	mu          sync.Mutex
	eventCount  int        // v0.1.0 R-372: 每 N=100 触发快照
	snapshotFn  func(goalID string) // v0.1.0: 快照回调
	// 三层防线状态（R-1037/R-1041）：
	globalSeq   int64      // 防线二：全局单调递增序号（WAL 写入器分配）
	prevHash    [32]byte   // 防线三：上一条目 SHA-256（genesis=全零 32B）
}

// New 创建一个 Store。
func New(baseDir string) *Store {
	return &Store{baseDir: baseDir}
}

// SetSnapshotCallback 设置快照回调（v0.1.0 R-372: N=100 定期快照）。
func (s *Store) SetSnapshotCallback(fn func(goalID string)) { s.snapshotFn = fn }

// Append 向 Goal 的 events.jsonl 追加一个事件。
// 调用 fsync 保证持久性。O_APPEND 保证 POSIX 原子写入。
//
// R-1384 (P-7 实测): 快照回调必须在 s.mu 临界区之外执行——daemon 注册的回调
// （main.go R-383 接线）会重入 SaveSnapshot（再次 s.mu.Lock()），而 Go sync.Mutex
// 不可重入：锁内调用回调即自锁死锁。因此锁内只完成事件文件追加并记录"是否需要
// 快照"标志，锁释放后再调用 snapshotFn。触发条件数值不变（每 N=100 事件）。
func (s *Store) Append(goalID string, evt events.Event) error {
	s.mu.Lock()

	dir := filepath.Join(s.baseDir, goalID)
	if err := os.MkdirAll(dir, 0755); err != nil {
		s.mu.Unlock()
		return fmt.Errorf("statestore: 创建目录失败: %w", err)
	}

	f, err := os.OpenFile(
		filepath.Join(dir, "events.jsonl"),
		os.O_APPEND|os.O_CREATE|os.O_WRONLY,
		0600,
	)
	if err != nil {
		s.mu.Unlock()
		return fmt.Errorf("statestore: 打开 events.jsonl 失败: %w", err)
	}

	// 三层完整性防线写入（R-1037/R-1041/R-1393/R-1373）：
	// 防线二：seq 单调性——global_seq 递增（WAL 写入器唯一分配，发布方不设 Seq）
	s.globalSeq++
	evt.Seq = int(s.globalSeq)
	// 防线三：hash chain——prev_hash=上一条目 SHA-256（genesis=全零 32B hex）
	if s.globalSeq == 1 {
		evt.PrevHash = "0000000000000000000000000000000000000000000000000000000000000000" // genesis
	} else {
		evt.PrevHash = fmt.Sprintf("%x", s.prevHash)
	}
	// 重新序列化（含 seq+prev_hash）——唯一一次 Marshal（R-1453：删除第一次 Marshal）
	var data []byte
	data, err = json.Marshal(evt)
	if err != nil {
		f.Close()
		s.mu.Unlock()
		return fmt.Errorf("statestore: 编码事件失败: %w", err)
	}
	// 防线一：CRC32 条目级校验和（计算输入=即将写入的确切字节——JSON 部分，R-1453）
	crc := crc32.ChecksumIEEE(data)
	// 防线三：prevHash 暂存（\n 前——R-1453 可读性修正）
	s.prevHash = sha256.Sum256(data)
	// WAL 行 envelope 格式（R-1453）：<json>\t<crc32_hex>\t<format_version>\n
	// format_version="1"——未来追加逐行字段的兼容锚点+区分"旧格式无 CRC 行"的唯一依据
	line := append(data, '\t')
	line = append(line, []byte(fmt.Sprintf("%08x", crc))...)
	line = append(line, '\t')
	line = append(line, '1')
	line = append(line, '\n')
	data = line

	if _, err := f.Write(data); err != nil {
		f.Close()
		s.mu.Unlock()
		return fmt.Errorf("statestore: 写入事件失败: %w", err)
	}
	if err := f.Sync(); err != nil {
		f.Close()
		s.mu.Unlock()
		return fmt.Errorf("statestore: fsync 失败: %w", err)
	}
	f.Close()

	s.eventCount++
	// v0.1.0 R-372: 每 N=100 事件触发快照（阈值不变）。
	needsSnapshot := s.eventCount%SnapshotInterval == 0 && s.snapshotFn != nil
	s.mu.Unlock()

	// R-1384: 回调移至锁外——允许回调重入 Store（SaveSnapshot/Append）。
	if needsSnapshot {
		s.snapshotFn(goalID)
	}
	return nil
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
		// R-1453 envelope 格式解析：<json>\t<crc32_hex>\t<format_version>\n
		// 读取方契约：先按 \t 拆分取 JSON 部分（seq 过滤+Unmarshal 用），
		// 完整行（含 \t 后缀）保留用于 hash chain 验证
		line := scanner.Bytes()
		jsonPart := line
		if idx := bytes.IndexByte(line, '\t'); idx >= 0 {
			jsonPart = line[:idx]
		}
		// 按 seq 过滤
		var evt struct{ Seq int `json:"seq"` }
		if err := json.Unmarshal(jsonPart, &evt); err != nil {
			continue // 非 JSON 行跳过（容错——旧格式兼容）
		}
		if evt.Seq > fromSeq {
			// 复制字节——scanner.Bytes() 是临时缓冲区（完整行含 envelope 后缀）
			full := make([]byte, len(line))
			copy(full, line)
			events = append(events, full)
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
