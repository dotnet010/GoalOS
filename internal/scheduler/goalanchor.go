// Package scheduler — GoalAnchor 计数器。
// Scheduler 维护每 Goal 的 LLM 调用计数。到达 N=20→触发 GoalAnchor 检查。
// 计数器在 PlanRequested 中注入 goal_anchor_check 标志。
//
// 设计依据：05 架构文档 §3、R169。
package scheduler

import "sync"

// GoalAnchorTracker 追踪每个 Goal 的 LLM 调用次数。
type GoalAnchorTracker struct {
	mu    sync.RWMutex
	count map[string]int // goalID → LLM 调用次数
	limit int             // 触发阈值。默认 20
}

// NewGoalAnchorTracker 创建 GoalAnchor 追踪器。
func NewGoalAnchorTracker(limit int) *GoalAnchorTracker {
	if limit <= 0 {
		limit = 20
	}
	return &GoalAnchorTracker{
		count: make(map[string]int),
		limit: limit,
	}
}

// Increment 增加指定 Goal 的 LLM 调用计数。
// 返回 true 表示达到阈值——应触发 GoalAnchor 检查。
func (g *GoalAnchorTracker) Increment(goalID string) bool {
	g.mu.Lock()
	defer g.mu.Unlock()

	g.count[goalID]++
	if g.count[goalID] >= g.limit {
		g.count[goalID] = 0 // 重置计数器
		return true
	}
	return false
}

// Reset 重置指定 Goal 的计数器。
func (g *GoalAnchorTracker) Reset(goalID string) {
	g.mu.Lock()
	defer g.mu.Unlock()
	g.count[goalID] = 0
}

// Count 返回当前计数。
func (g *GoalAnchorTracker) Count(goalID string) int {
	g.mu.RLock()
	defer g.mu.RUnlock()
	return g.count[goalID]
}
