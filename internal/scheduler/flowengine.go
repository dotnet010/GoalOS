// Package scheduler — Flow Engine v0.1.0。
// FlowRegistry: 模板注册与查找。FlowComposer: 约束验证与降级。
// PlanningCircuitBreaker: 规划层熔断。
//
// 设计依据：05 架构文档 §3.11、R317。

package scheduler

import (
	"fmt"
	"log"
	"sort"
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

// FlowRegistry 管理 Flow 模板的注册和查找（v0.1.0）。
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
		Name:        "builtin/research-v1",
		Version:     "1.0",
		Description: "调研任务流程（R-362: 补全 5 模板）",
		TaskTypes:   []string{"research"},
		Stages: []FlowStage{
			{Name: "信息收集", Order: 1, Required: true},
			{Name: "信息整理", Order: 2, Required: true},
			{Name: "分析报告", Order: 3, Required: true},
		},
		FailurePolicy: "continue_on_warn",
	})
	fr.Register(&FlowTemplate{
		Name:        "builtin/content-create-v1",
		Version:     "1.0",
		Description: "内容创作任务流程（R-362: 补全 5 模板）",
		TaskTypes:   []string{"content_creation"},
		Stages: []FlowStage{
			{Name: "素材收集", Order: 1, Required: true},
			{Name: "内容生成", Order: 2, Required: true},
			{Name: "审核校验", Order: 3, Required: true},
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

// FlowComposer 是 Flow 约束验证与降级控制器（v0.1.0）。
type FlowComposer struct {
	registry       *FlowRegistry
	mu             sync.Mutex        // G5: rejectCount 并发保护
	rejectCount    map[string]int    // goalID → 连续拒绝次数
}

// NewFlowComposer 创建 FlowComposer。
func NewFlowComposer(registry *FlowRegistry) *FlowComposer {
	return &FlowComposer{
		registry:    registry,
		rejectCount: make(map[string]int),
	}
}

// MatchFlow 根据 TaskAnalysis 匹配最佳 Flow（v0.1.0 规则硬匹配）。
func (fc *FlowComposer) MatchFlow(taskType string) (*FlowTemplate, error) {
	candidates := fc.registry.ListByTaskType(taskType)
	if len(candidates) == 0 {
		// 无匹配——使用 generic-v1
		return fc.registry.Lookup("builtin/generic-v1")
	}
	// R-831: 按名称匹配度排序——精确 > 前缀 > 兜底
	if len(candidates) > 1 {
		sort.Slice(candidates, func(i, j int) bool {
			return flowMatchScore(candidates[i].Name, taskType) > flowMatchScore(candidates[j].Name, taskType)
		})
		names := make([]string, len(candidates))
		for i, c := range candidates { names[i] = c.Name }
		log.Printf("[FlowComposer] %d candidates for %s: %v — using %s", len(candidates), taskType, names, candidates[0].Name)
	}
	return candidates[0], nil
}

// flowMatchScore 计算 Flow 模板名称与 taskType 的匹配度（R-831）。
func flowMatchScore(flowName, taskType string) int {
	if strings.Contains(flowName, taskType) { return 100 } // 精确包含
	if strings.Contains(flowName, strings.Split(taskType, "-")[0]) { return 50 }
	return 0
}

// ValidatePlan 验证 MissionGraph 是否覆盖 Flow 的 required stages。
// 校验失败→MissionGraphRejected→Agent 修正。连续 3 次→降级。
func (fc *FlowComposer) ValidatePlan(goalID string, flow *FlowTemplate, coveredStageIDs []string) error {
	missing := fc.findMissingStages(flow, coveredStageIDs)
	if len(missing) == 0 {
		fc.rejectCount[goalID] = 0 // 重置计数器
		return nil
	}

	fc.mu.Lock()
	fc.rejectCount[goalID]++
	rejectN := fc.rejectCount[goalID]
	fc.mu.Unlock()
	log.Printf("[FlowComposer] goal=%s missing stages: %v (reject #%d)",
		goalID, missing, rejectN)

	// 连续 3 次→降级
	if rejectN >= 3 {
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

// PlanningCircuitBreaker 规划层熔断（v0.1.0）。
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

// ─── K1: 对抗性欺骗检测（R-818）─────────────────────────────

// DetectDeceptivePattern 检测常见 LLM 欺骗模式。
// shell.execute("echo skip")——伪装成执行但实际跳过。
func (fc *FlowComposer) DetectDeceptivePattern(actionType, target string) bool {
	deceptiveTargets := []string{
		"echo skip", "echo done", "echo ok",
		"sleep 0", "sleep 1",
		"true", "false", "exit 0",
	}
	if actionType == "shell.execute" {
		for _, d := range deceptiveTargets {
			if target == d {
				return true
			}
		}
	}
	return false
}

// DetectMismatchedAction 检测 action_type 与 stage 不匹配。
func (fc *FlowComposer) DetectMismatchedAction(stage, actionType string) bool {
	validActions := map[string][]string{
		"code_generation": {"shell.execute", "fs.write", "browser.open"},
		"code_review":     {"fs.read", "web.search"},
		"testing":         {"shell.execute", "fs.read"},
	}
	if allowed, ok := validActions[stage]; ok {
		for _, a := range allowed {
			if a == actionType {
				return false // 合法匹配
			}
		}
		return true // 不在允许列表中
	}
	return false
}

// DetectEmptyParams 检测空参数列表——什么都没做。
func (fc *FlowComposer) DetectEmptyParams(params map[string]interface{}) bool {
	return len(params) == 0
}
