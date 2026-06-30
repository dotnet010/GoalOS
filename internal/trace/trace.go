// Package trace — GoalOS 运行时链路追踪。
// 在每个 Pipeline 环节记录时间戳、状态、耗时。定位故障点无需翻日志。
// R-726: 内部运行时观测——每个环节必须有检测点。
package trace

import (
	"fmt"
	"log"
	"sync"
	"time"
)

// Stage 表示 Pipeline 的一个阶段。
type Stage struct {
	Name   string
	GoalID string
	Start  time.Time
	Status string // "started" | "ok" | "failed"
	Err    error
}

// Tracer 追踪一个 Goal 的完整 Pipeline 执行链路。
type Tracer struct {
	mu     sync.Mutex
	goalID string
	stages []Stage
}

var (
	active   = make(map[string]*Tracer)
	activeMu sync.Mutex
)

// Start 开始追踪一个 Goal。
func Start(goalID string) *Tracer {
	t := &Tracer{goalID: goalID}
	activeMu.Lock()
	active[goalID] = t
	activeMu.Unlock()
	log.Printf("[TRACE] %s ── Pipeline Start ──", goalID)
	return t
}

// StageStart 标记阶段开始。
// [FIXED] 返回 error，让调用方知道是否成功记录
func (t *Tracer) StageStart(name string) error {
	if t == nil {
		// [FIXED] 原代码：静默忽略 nil Tracer 调用
		// [FIXED] 现在：返回错误，强制调用方处理
		return fmt.Errorf("trace.StageStart: Tracer is nil (goalID unknown)")
	}
	if name == "" {
		// [FIXED] 新增：防止空阶段名
		return fmt.Errorf("trace.StageStart: stage name cannot be empty")
	}

	t.mu.Lock()
	defer t.mu.Unlock()

	// [FIXED] 新增：检测重复开始的阶段
	for _, s := range t.stages {
		if s.Name == name && s.Status == "started" {
			return fmt.Errorf("trace.StageStart: stage %q already started (possible duplicate call)", name)
		}
	}

	t.stages = append(t.stages, Stage{
		Name:   name,
		GoalID: t.goalID,
		Start:  time.Now(),
		Status: "started",
	})
	log.Printf("[TRACE] %s ├─ %s ...", t.goalID, name)
	return nil
}

// StageOK 标记阶段成功。
// [FIXED] 返回 error，如果阶段未开始或重复结束，调用方必须处理
func (t *Tracer) StageOK(name string) error {
	if t == nil {
		// [FIXED] 原代码：静默忽略 nil Tracer 调用
		// [FIXED] 现在：返回错误，强制调用方处理
		return fmt.Errorf("trace.StageOK: Tracer is nil (goalID unknown)")
	}
	if name == "" {
		return fmt.Errorf("trace.StageOK: stage name cannot be empty")
	}

	t.mu.Lock()
	defer t.mu.Unlock()

	// [FIXED] 原代码：如果找不到对应阶段，静默返回（欺骗性虚假返回）
	// [FIXED] 现在：找不到时返回错误，让调用方知道记录失败
	found := false
	for i := len(t.stages) - 1; i >= 0; i-- {
		if t.stages[i].Name == name && t.stages[i].Status == "started" {
			elapsed := time.Since(t.stages[i].Start)
			t.stages[i].Status = "ok"
			log.Printf("[TRACE] %s ├─ %s ✅ (%v)", t.goalID, name, elapsed.Round(time.Millisecond))
			found = true
			return nil
		}
	}

	if !found {
		// [FIXED] 关键修复：阶段未开始或已结束，返回真实错误
		return fmt.Errorf("trace.StageOK: stage %q not found or already completed (possible StageStart missed or StageOK/StageFail called twice)", name)
	}
	return nil
}

// StageFail 标记阶段失败。
// [FIXED] 返回 error，如果阶段未开始或重复结束，调用方必须处理
func (t *Tracer) StageFail(name string, err error) error {
	if t == nil {
		// [FIXED] 原代码：静默忽略 nil Tracer 调用
		// [FIXED] 现在：返回错误，强制调用方处理
		return fmt.Errorf("trace.StageFail: Tracer is nil (goalID unknown)")
	}
	if name == "" {
		return fmt.Errorf("trace.StageFail: stage name cannot be empty")
	}

	t.mu.Lock()
	defer t.mu.Unlock()

	// [FIXED] 原代码：如果找不到对应阶段，静默返回（欺骗性虚假返回）
	// [FIXED] 现在：找不到时返回错误，让调用方知道记录失败
	found := false
	for i := len(t.stages) - 1; i >= 0; i-- {
		if t.stages[i].Name == name && t.stages[i].Status == "started" {
			elapsed := time.Since(t.stages[i].Start)
			t.stages[i].Status = "failed"
			t.stages[i].Err = err
			log.Printf("[TRACE] %s ├─ %s ❌ (%v) — %v", t.goalID, name, elapsed.Round(time.Millisecond), err)
			found = true
			return nil
		}
	}

	if !found {
		// [FIXED] 关键修复：阶段未开始或已结束，返回真实错误
		return fmt.Errorf("trace.StageFail: stage %q not found or already completed (possible StageStart missed or StageOK/StageFail called twice)", name)
	}
	return nil
}

// Summary 输出完整 Pipeline 摘要。
// [FIXED] 返回 error，如果追踪器异常（如无任何阶段、或有阶段未结束），调用方必须处理
func (t *Tracer) Summary() error {
	if t == nil {
		// [FIXED] 原代码：静默处理 nil Tracer
		// [FIXED] 现在：返回错误
		return fmt.Errorf("trace.Summary: Tracer is nil")
	}

	t.mu.Lock()
	defer t.mu.Unlock()

	var total time.Duration
	failures := 0
	incomplete := 0 // [FIXED] 新增：统计未结束的阶段

	for _, s := range t.stages {
		if s.Status == "failed" {
			failures++
		}
		if s.Status == "started" {
			// [FIXED] 新增：检测未结束的阶段（只 started 未 ok/fail）
			incomplete++
		}
	}
	if len(t.stages) > 0 {
		total = time.Since(t.stages[0].Start)
	}

	// [FIXED] 新增：如果存在未结束的阶段，视为异常
	if incomplete > 0 {
		log.Printf("[TRACE] %s ── Pipeline ⚠️ INCOMPLETE (%d stages still running, %d failed, total %v, %d stages) ──",
			t.goalID, incomplete, failures, total.Round(time.Millisecond), len(t.stages))
		activeMu.Lock()
		delete(active, t.goalID)
		activeMu.Unlock()
		return fmt.Errorf("trace.Summary: %d stage(s) still in 'started' state (missing StageOK/StageFail call)", incomplete)
	}

	status := "✅ COMPLETED"
	if failures > 0 {
		status = fmt.Sprintf("❌ FAILED (%d stages failed)", failures)
	}
	log.Printf("[TRACE] %s ── Pipeline %s (total %v, %d stages) ──", t.goalID, status, total.Round(time.Millisecond), len(t.stages))

	activeMu.Lock()
	delete(active, t.goalID)
	activeMu.Unlock()
	return nil
}

// Get 获取活跃追踪器。
// [FIXED] 返回 error，让调用方知道追踪器是否存在
func Get(goalID string) (*Tracer, error) {
	activeMu.Lock()
	defer activeMu.Unlock()
	t, ok := active[goalID]
	if !ok {
		// [FIXED] 原代码：返回 nil，调用方无法区分"不存在"和"存在但为空"
		// [FIXED] 现在：返回错误，明确告知追踪器不存在
		return nil, fmt.Errorf("trace.Get: no active tracer found for goalID %q", goalID)
	}
	return t, nil
}

// MustGet 是 Get 的便捷版本，如果追踪器不存在则 panic。
// 用于确定追踪器必须存在的场景（如内部断言）。
func MustGet(goalID string) *Tracer {
	t, err := Get(goalID)
	if err != nil {
		panic(err)
	}
	return t
}

// ActiveGoalIDs 返回当前所有活跃追踪的 GoalID 列表。
// [ADDED] 新增：便于调试和检测内存泄漏
func ActiveGoalIDs() []string {
	activeMu.Lock()
	defer activeMu.Unlock()
	ids := make([]string, 0, len(active))
	for id := range active {
		ids = append(ids, id)
	}
	return ids
}

// StageCount 返回当前已记录的阶段数。
// [ADDED] 新增：便于调试和验证
func (t *Tracer) StageCount() int {
	if t == nil {
		return 0
	}
	t.mu.Lock()
	defer t.mu.Unlock()
	return len(t.stages)
}
