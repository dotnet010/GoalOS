// Package scheduler — v0.2.0 Week 7: B7b SSE + B8 预估 + B9 failHints + B10 可配置
package scheduler

import (
	"fmt"
	"strings"
	"sync"
	"time"
)

// ─── B7b: SSE Hub ──────────────────────────────────────────────

// SSEHub 管理 SSE 客户端连接和事件广播。
type SSEHub struct {
	mu      sync.Mutex
	clients map[string]chan string // clientID → event channel
	nextID  int
}

// NewSSEHub 创建 SSE Hub。
func NewSSEHub() *SSEHub {
	return &SSEHub{
		clients: make(map[string]chan string),
	}
}

// Connect 新客户端连接。返回客户端 ID。
func (h *SSEHub) Connect() string {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.nextID++
	id := fmt.Sprintf("sse-%d", h.nextID)
	h.clients[id] = make(chan string, 100) // buffer 100 events
	return id
}

// Disconnect 客户端断开。清理资源。
func (h *SSEHub) Disconnect(clientID string) {
	h.mu.Lock()
	defer h.mu.Unlock()
	if ch, ok := h.clients[clientID]; ok {
		close(ch)
		delete(h.clients, clientID)
	}
}

// Broadcast 向所有客户端广播事件。
func (h *SSEHub) Broadcast(eventType, data string) {
	h.mu.Lock()
	defer h.mu.Unlock()
	for _, ch := range h.clients {
		select {
		case ch <- fmt.Sprintf("event: %s\ndata: %s\n\n", eventType, data):
		default:
			// 客户端 channel 满——丢弃（避免阻塞广播）
		}
	}
}

// ClientCount 返回当前连接数。
func (h *SSEHub) ClientCount() int {
	h.mu.Lock()
	defer h.mu.Unlock()
	return len(h.clients)
}

// PendingCount 返回客户端待发送事件数。
func (h *SSEHub) PendingCount(clientID string) int {
	h.mu.Lock()
	defer h.mu.Unlock()
	if ch, ok := h.clients[clientID]; ok {
		return len(ch)
	}
	return 0
}

// ─── B8: Goal 创建预估时间 ────────────────────────────────────

// GoalEstimate Goal 创建的预估结果。
type GoalEstimate struct {
	Duration   time.Duration
	NextStatus string
}

// EstimateGoalDuration 根据目标描述预估完成时间。
func EstimateGoalDuration(goal string) GoalEstimate {
	complexity := classifyComplexity(goal)
	goalLen := len([]rune(goal))

	baseTime := 5 * time.Second
	if complexity == "complex" || goalLen > 50 {
		baseTime = 60 * time.Second
	} else if complexity == "medium" || goalLen > 20 {
		baseTime = 15 * time.Second
	}

	return GoalEstimate{
		Duration:   baseTime,
		NextStatus: "Aligning",
	}
}

func classifyComplexity(goal string) string {
	complexKeywords := []string{"开发", "系统", "完整", "CRM", "ERP", "平台", "架构"}
	simpleKeywords := []string{"查询", "天气", "时间", "计算", "翻译"}

	for _, kw := range complexKeywords {
		if strings.Contains(goal, kw) {
			return "complex"
		}
	}
	for _, kw := range simpleKeywords {
		if strings.Contains(goal, kw) {
			return "simple"
		}
	}
	return "medium"
}

// ─── B9: failHints 全量映射 ────────────────────────────────────

// FailHint 单个错误提示。
type FailHint struct {
	Code       string
	Suggestion string
	Buttons    []string
}

var failHintsMap = map[string]FailHint{
	"TIMEOUT": {
		Code:       "TIMEOUT",
		Suggestion: "模型响应超时。建议换更快的模型或稍后重试。",
		Buttons:    []string{"更换模型", "继续等待"},
	},
	"PARSE_ERROR": {
		Code:       "PARSE_ERROR",
		Suggestion: "AI 返回了无法解析的结果。这通常是模型兼容性问题。",
		Buttons:    []string{"更换模型", "放弃目标"},
	},
	"LLM_ERROR": {
		Code:       "LLM_ERROR",
		Suggestion: "AI 服务暂时不可用。请检查网络或 API Key 配置。",
		Buttons:    []string{"重试", "更换模型"},
	},
	"INVALID_OUTPUT": {
		Code:       "INVALID_OUTPUT",
		Suggestion: "AI 产出的结果不符合预期格式。系统将用新 Session 重新执行。",
		Buttons:    []string{"简化方案", "更换模型"},
	},
	"IPC_ERROR": {
		Code:       "IPC_ERROR",
		Suggestion: "系统内部通信异常。可能是插件兼容性问题。",
		Buttons:    []string{"重试", "放弃目标"},
	},
	"SECCOMP_VIOLATION": {
		Code:       "SECCOMP_VIOLATION",
		Suggestion: "检测到异常操作被安全策略拦截。目标已暂停。",
		Buttons:    []string{"查看详情", "调整安全级别", "放弃目标"},
	},
	"POLICY_DENIED": {
		Code:       "POLICY_DENIED",
		Suggestion: "操作被安全策略拒绝。如需继续请调整安全级别。",
		Buttons:    []string{"调整安全级别", "放弃目标"},
	},
	"BUDGET_EXCEEDED": {
		Code:       "BUDGET_EXCEEDED",
		Suggestion: "Token 预算已用完。可以追加预算后继续。",
		Buttons:    []string{"追加预算", "简化方案", "放弃目标"},
	},
	"EXEC_TIMEOUT": {
		Code:       "EXEC_TIMEOUT",
		Suggestion: "操作执行超时。可能是任务过于复杂。",
		Buttons:    []string{"简化方案", "重试"},
	},
	"EXEC_CRASH": {
		Code:       "EXEC_CRASH",
		Suggestion: "执行引擎意外崩溃。系统已自动保存进度。",
		Buttons:    []string{"重试", "简化方案", "放弃目标"},
	},
}

// GetAllFailHints 返回全量 failHints 映射。
func GetAllFailHints() map[string]FailHint {
	return failHintsMap
}

// GetFailHint 获取单个错误提示。
func GetFailHint(code string) FailHint {
	if h, ok := failHintsMap[code]; ok {
		return h
	}
	return FailHint{Code: code, Suggestion: "未知错误", Buttons: []string{"放弃目标"}}
}

// ─── B10: 自动确认可配置 ──────────────────────────────────────

// AutoConfirmConfig MissionGraph 自动确认配置。
type AutoConfirmConfig struct {
	Mode string // "autonomous" | "manual" | ""
}

// ShouldAutoConfirm 判断是否应自动确认 MissionGraph。
func (c AutoConfirmConfig) ShouldAutoConfirm() bool {
	return c.Mode == "autonomous"
}
