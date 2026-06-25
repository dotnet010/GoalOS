// Package scheduler — Flow Engine v1.1.0。
// FlowRegistry: 模板注册与查找。FlowComposer: 约束验证与降级。
// PlanningCircuitBreaker: 规划层熔断。
//
// 设计依据：05 架构文档 §3.11、R317。

package scheduler

import (
	"fmt"
	"log"
	"strings"
	"sync"
)

// FlowTemplate 是声明式执行流程模板。
type FlowTemplate struct {
	Name        string      `json:"name"`         // "builtin/code-project-v1"
	Version     string      `json:"version"`      // "1.0"
	Description string      `json:"description"`  // 自然语言描述
	TaskTypes   []string    `json:"applicable_task_types"` // 适用的任务类型
	Stages      []FlowStage `json:"stages"`        // 阶段序列
	FailurePolicy string    `json:"failure_policy"` // "fail_fast"|"continue_on_warn"
}

// FlowStage 是 Flow 的一个阶段。
type FlowStage struct {
	Name     string `json:"name"`      // "需求分析"
	Order    int    `json:"order"`     // 阶段序号
	Required bool   `json:"required"`  // true=强制阶段。Agent.Plan() 必须覆盖
}

// FlowRegistry 管理 Flow 模板的注册和查找（v1.1.0）。
type FlowRegistry struct {
	mu      sync.RWMutex
	flows   map[string]*FlowTemplate // name → template
}

// NewFlowRegistry 创建 FlowRegistry 并加载内置模板。
func NewFlowRegistry() *FlowRegistry {
	fr := &FlowRegistry{flows: make(map[string]*FlowTemplate)}
	fr.loadBuiltins()
	return fr
}

// Register 注册一个 Flow 模板。
func (fr *FlowRegistry) Register(flow *FlowTemplate) error {
	fr.mu.Lock()
	defer fr.mu.Unlock()
	if flow.Name == "" {
		return fmt.Errorf("flow: name is required")
	}
	fr.flows[flow.Name] = flow
	log.Printf("[FlowRegistry] registered %s v%s", flow.Name, flow.Version)
	return nil
}

// Lookup 按名称查找 Flow 模板。
func (fr *FlowRegistry) Lookup(name string) (*FlowTemplate, error) {
	fr.mu.RLock()
	defer fr.mu.RUnlock()
	flow, ok := fr.flows[name]
	if !ok {
		return nil, fmt.Errorf("flow: %s not found", name)
	}
	return flow, nil
}

// ListByTaskType 列出适用于指定任务类型的所有 Flow。
func (fr *FlowRegistry) ListByTaskType(taskType string) []*FlowTemplate {
	fr.mu.RLock()
	defer fr.mu.RUnlock()
	var result []*FlowTemplate
	for _, f := range fr.flows {
		for _, tt := range f.TaskTypes {
			if tt == taskType {
				result = append(result, f)
				break
			}
		}
	}
	return result
}

// loadBuiltins 加载内置 Flow 模板。
func (fr *FlowRegistry) loadBuiltins() {
	fr.Register(&FlowTemplate{
		Name:        "builtin/code-project-v1",
		Version:     "1.0",
		Description: "标准代码项目开发流程",
		TaskTypes:   []string{"code_generation"},
		Stages: []FlowStage{
			{Name: "需求分析", Order: 1, Required: true},
			{Name: "架构设计", Order: 2, Required: true},
			{Name: "代码生成", Order: 3, Required: true},
			{Name: "测试", Order: 4, Required: true},
			{Name: "部署", Order: 5, Required: false},
		},
		FailurePolicy: "fail_fast",
	})
	fr.Register(&FlowTemplate{
		Name:        "builtin/data-analysis-v1",
		Version:     "1.0",
		Description: "数据分析任务流程",
		TaskTypes:   []string{"data_analysis"},
		Stages: []FlowStage{
			{Name: "数据收集", Order: 1, Required: true},
			{Name: "数据清洗", Order: 2, Required: true},
			{Name: "分析建模", Order: 3, Required: true},
			{Name: "报告生成", Order: 4, Required: true},
		},
		FailurePolicy: "continue_on_warn",
	})
	fr.Register(&FlowTemplate{
		Name:        "builtin/generic-v1",
		Version:     "1.0",
		Description: "通用兜底流程。Flow 降级时使用",
		TaskTypes:   []string{"generic"},
		Stages:      []FlowStage{},  // 无强制阶段——自由规划
		FailurePolicy: "continue_on_warn",
	})
}

// FlowComposer 是 Flow 约束验证与降级控制器（v1.1.0）。
type FlowComposer struct {
	registry       *FlowRegistry
	rejectCount    map[string]int // goalID → 连续拒绝次数
}

// NewFlowComposer 创建 FlowComposer。
func NewFlowComposer(registry *FlowRegistry) *FlowComposer {
	return &FlowComposer{
		registry:    registry,
		rejectCount: make(map[string]int),
	}
}

// MatchFlow 根据 TaskAnalysis 匹配最佳 Flow（v1.1.0 规则硬匹配）。
func (fc *FlowComposer) MatchFlow(taskType string) (*FlowTemplate, error) {
	candidates := fc.registry.ListByTaskType(taskType)
	if len(candidates) == 0 {
		// 无匹配——使用 generic-v1
		return fc.registry.Lookup("builtin/generic-v1")
	}
	// v1.1.0: 规则硬匹配——返回第一个匹配的
	return candidates[0], nil
}

// ValidatePlan 验证 MissionGraph 是否覆盖 Flow 的 required stages。
// 校验失败→MissionGraphRejected→Agent 修正。连续 3 次→降级。
func (fc *FlowComposer) ValidatePlan(goalID string, flow *FlowTemplate, coveredStageIDs []string) error {
	missing := fc.findMissingStages(flow, coveredStageIDs)
	if len(missing) == 0 {
		fc.rejectCount[goalID] = 0 // 重置计数器
		return nil
	}

	fc.rejectCount[goalID]++
	log.Printf("[FlowComposer] goal=%s missing stages: %v (reject #%d)",
		goalID, missing, fc.rejectCount[goalID])

	// 连续 3 次→降级
	if fc.rejectCount[goalID] >= 3 {
		log.Printf("[FlowComposer] goal=%s degraded to generic-v1 after %d rejects", goalID, fc.rejectCount[goalID])
		fc.rejectCount[goalID] = 0
		return &FlowDegradeError{
			Message:      fmt.Sprintf("标准流程无法覆盖目标需求（缺失阶段: %s）。已切换为自定义路径", strings.Join(missing, ", ")),
			DegradedFlow: "builtin/generic-v1",
		}
	}

	return &FlowValidateError{
		MissingStages: missing,
		Message:       fmt.Sprintf("遗漏 required stages: %s", strings.Join(missing, ", ")),
	}
}

// findMissingStages 找出未被覆盖的 required stages。
func (fc *FlowComposer) findMissingStages(flow *FlowTemplate, covered []string) []string {
	coveredSet := make(map[string]bool)
	for _, s := range covered {
		coveredSet[s] = true
	}
	var missing []string
	for _, stage := range flow.Stages {
		if stage.Required && !coveredSet[stage.Name] {
			missing = append(missing, stage.Name)
		}
	}
	return missing
}

// FlowValidateError 表示 Flow 验证失败。
type FlowValidateError struct {
	MissingStages []string
	Message       string
}

func (e *FlowValidateError) Error() string { return e.Message }

// FlowDegradeError 表示 Flow 已降级为 generic-v1。
type FlowDegradeError struct {
	Message      string
	DegradedFlow string
}

func (e *FlowDegradeError) Error() string { return e.Message }

// PlanningCircuitBreaker 规划层熔断（v1.1.0）。
type PlanningCircuitBreaker struct {
	planFailures map[string]int // goalID → 连续 Plan 失败次数
	maxFailures  int            // 触发熔断的阈值（默认 3）
}

// NewPlanningCircuitBreaker 创建规划层熔断器。
func NewPlanningCircuitBreaker() *PlanningCircuitBreaker {
	return &PlanningCircuitBreaker{
		planFailures: make(map[string]int),
		maxFailures:  3,
	}
}

// RecordFailure 记录一次 Plan 失败。返回 true 表示触发熔断。
func (pcb *PlanningCircuitBreaker) RecordFailure(goalID string) bool {
	pcb.planFailures[goalID]++
	return pcb.planFailures[goalID] >= pcb.maxFailures
}

// RecordSuccess 记录 Plan 成功。重置计数器。
func (pcb *PlanningCircuitBreaker) RecordSuccess(goalID string) {
	pcb.planFailures[goalID] = 0
}

// IsTripped 检查是否已熔断。
func (pcb *PlanningCircuitBreaker) IsTripped(goalID string) bool {
	return pcb.planFailures[goalID] >= pcb.maxFailures
}
