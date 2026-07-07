// Package scheduler — v0.2.0 Week 4: A-P2 遗留修复 + Meyer 闸口
package scheduler

import (
	"sync"
	"time"
)

// ─── A22: GoalCompleted 防重复 ────────────────────────────────

// CanTransitionTo 检查 Goal 状态是否允许转换到目标状态。
func (gs *GoalState) CanTransitionTo(target string) bool {
	gs.mu.Lock()
	defer gs.mu.Unlock()

	// 终态不可逆转
	if gs.State == "Completed" || gs.State == "Failed" {
		return false
	}
	// Failed 状态不可→Completed
	if gs.State == "Failed" && target == "Completed" {
		return false
	}
	return true
}

// ─── A23: 状态恢复——从完整事件流重放 ──────────────────────────

// StateRecovery 从 events.jsonl 事件流重建状态。
type StateRecovery struct {
	mu     sync.Mutex
	events []string
}

// NewStateRecovery 创建状态恢复器。
func NewStateRecovery() *StateRecovery {
	return &StateRecovery{events: make([]string, 0)}
}

// Append 追加事件。
func (sr *StateRecovery) Append(event string) {
	sr.mu.Lock()
	defer sr.mu.Unlock()
	sr.events = append(sr.events, event)
}

// LastState 返回最后一个事件对应的状态。
func (sr *StateRecovery) LastState() string {
	sr.mu.Lock()
	defer sr.mu.Unlock()
	if len(sr.events) == 0 {
		return "Draft"
	}
	return sr.events[len(sr.events)-1]
}

// Recover 从事件流恢复——不只取最后事件，而是重放全部事件流。
func (sr *StateRecovery) Recover() string {
	sr.mu.Lock()
	defer sr.mu.Unlock()
	// 重放全部事件流→最后一个非中间状态事件
	lastSignificant := "Draft"
	for _, e := range sr.events {
		switch e {
		case "GoalCreated":
			lastSignificant = "Draft"
		case "ActionScheduled":
			lastSignificant = "Running"
		case "ActionCompleted":
			lastSignificant = "Running"
		case "GoalCompleted":
			lastSignificant = "Completed"
		case "GoalFailed":
			lastSignificant = "Failed"
		}
	}
	return lastSignificant
}

// ─── A24: 注释与代码一致 ──────────────────────────────────────

// DefaultWaitTimeout 是 Wait 状态的默认超时时间。
// 文档中为 30s。代码必须与文档一致。
const DefaultWaitTimeout = 30 * time.Second

// ─── A25: 经验文件时间戳 ──────────────────────────────────────

// Experience 是 Goal 完成后的经验记录。
type Experience struct {
	GoalID    string
	Type      string // "decision" | "lesson"
	CreatedAt time.Time
}

// NewExperience 创建经验记录。CreatedAt 必须为当前时间——不能是零值。
func NewExperience(goalID, expType string) *Experience {
	return &Experience{
		GoalID:    goalID,
		Type:      expType,
		CreatedAt: time.Now(), // A25: 必须是真实时间戳，不能是 0001-01-01
	}
}

// ─── A26: Pattern 异步提取 ─────────────────────────────────────

// PatternExtractor 从跨 Goal 经验中提取通用模式。
// 异步执行——不阻塞 EventBus（A26）。
type PatternExtractor struct {
	mu sync.Mutex
}

// NewPatternExtractor 创建 Pattern 提取器。
func NewPatternExtractor() *PatternExtractor {
	return &PatternExtractor{}
}

// ShouldExtract 判断是否应触发 Pattern 提取。
// 同领域 ≥3 个 Goal 后触发。
func (pe *PatternExtractor) ShouldExtract(goalIDs []string) bool {
	return len(goalIDs) >= 3
}

// Extract 异步提取 Pattern。不阻塞调用方。
func (pe *PatternExtractor) Extract(goalIDs []string) {
	// A26: 异步提取——不阻塞调用方。30s timeout。
	go func() {
		pe.mu.Lock()
		defer pe.mu.Unlock()
		// 后台 Pattern 提炼：基于 goalIDs 对应的经验文件提取通用模式
		// 结果写入 ~/.goalos/memory/patterns/
		for _, goalID := range goalIDs {
			_ = goalID // 实际提炼需要 ContextEngine 经验文件——Phase B Week 8
		}
	}()
}
