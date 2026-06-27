// Package metrics 实现 GoalOS 运行时指标采集与 Prometheus 格式导出（v0.1.0 H8）。
//
// 核心指标（05架构 §3.5）:
//   - pipeline_duration_ms: PipelineRunner 每阶段耗时
//   - check_results_total: Check 原语结果分布
//   - decide_paths_total: Decide 路径分布
//   - active_goals: 当前活跃 Goal 数
//   - token_usage_total: Token 消耗总计
//
// 设计依据: 05架构 §3.5、H8、R-349。
package metrics

import (
	"fmt"
	"strings"
	"sync"
	"sync/atomic"
)

// Registry 是线程安全的指标注册表。
type Registry struct {
	mu       sync.RWMutex
	counters map[string]*atomic.Int64
	gauges   map[string]*atomic.Int64
}

// New 创建指标注册表。
func New() *Registry {
	return &Registry{
		counters: make(map[string]*atomic.Int64),
		gauges:   make(map[string]*atomic.Int64),
	}
}

// Counter 返回或创建计数器。
func (r *Registry) Counter(name string) *atomic.Int64 {
	r.mu.RLock()
	c, ok := r.counters[name]
	r.mu.RUnlock()
	if ok {
		return c
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if c, ok = r.counters[name]; ok {
		return c
	}
	c = &atomic.Int64{}
	r.counters[name] = c
	return c
}

// Gauge 返回或创建仪表。
func (r *Registry) Gauge(name string) *atomic.Int64 {
	r.mu.RLock()
	g, ok := r.gauges[name]
	r.mu.RUnlock()
	if ok {
		return g
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if g, ok = r.gauges[name]; ok {
		return g
	}
	g = &atomic.Int64{}
	r.gauges[name] = g
	return g
}

// PrometheusText 以 Prometheus 文本格式导出所有指标。
func (r *Registry) PrometheusText() string {
	var b strings.Builder
	b.WriteString("# HELP goalos_info GoalOS daemon info\n")
	b.WriteString("# TYPE goalos_info gauge\n")

	r.mu.RLock()
	defer r.mu.RUnlock()

	for name, g := range r.gauges {
		fmt.Fprintf(&b, "goalos_%s %d\n", name, g.Load())
	}
	for name, c := range r.counters {
		fmt.Fprintf(&b, "# TYPE goalos_%s counter\n", name)
		fmt.Fprintf(&b, "goalos_%s %d\n", name, c.Load())
	}
	return b.String()
}

// ── 预定义指标名称（05架构 §3.5）──

const (
	MetricPipelineDurationMs  = "pipeline_duration_ms"
	MetricCheckResultsPass    = "check_results_pass_total"
	MetricCheckResultsWarn    = "check_results_warn_total"
	MetricCheckResultsBlock   = "check_results_block_total"
	MetricCheckResultsReject  = "check_results_reject_total"
	MetricDecideContinue      = "decide_paths_continue_total"
	MetricDecideRetry         = "decide_paths_retry_total"
	MetricDecideReplan        = "decide_paths_replan_total"
	MetricDecideEscalate      = "decide_paths_escalate_total"
	MetricDecideAbort         = "decide_paths_abort_total"
	MetricActiveGoals         = "active_goals"
	MetricTokenUsageTotal     = "token_usage_total"
	MetricEventsPublished     = "events_published_total"
	MetricActionsCompleted    = "actions_completed_total"
	MetricActionsFailed       = "actions_failed_total"
	MetricRecoveryRetries     = "recovery_retries_total"
	MetricAsyncEventsDropped  = "async_events_dropped_total"
)
