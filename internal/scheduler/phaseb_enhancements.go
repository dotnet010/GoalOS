// Package scheduler — v0.2.0 Phase B Week 5-6: B4-B6 + I8-I9
package scheduler

import (
	"encoding/json"
	"strings"
)

// ─── B4: CheckResult 枚举验证 ─────────────────────────────────

// validCheckResults 是所有合法的 CheckResult 值。
var validCheckResults = map[string]bool{
	"PASS":   true,
	"WARN":   true,
	"BLOCK":  true,
	"REJECT": true,
}

// IsValidCheckResult 验证 CheckResult 是否为合法枚举值。
// B4: switch 不验证 check() 返回合法枚举值→现在强制验证。
func IsValidCheckResult(result string) bool {
	return validCheckResults[result]
}

// ─── B5: LLM ToolCalls 防御 ────────────────────────────────────

// ClassifyToolCallError 分类 LLM ToolCall 错误。
// B5: Arguments 空字符串→INVALID_OUTPUT。非法 JSON→PARSE_ERROR。
func ClassifyToolCallError(actionType, arguments string) string {
	if arguments == "" {
		return "INVALID_OUTPUT"
	}
	if actionType == "" {
		return "INVALID_OUTPUT"
	}
	// 验证 JSON 合法性
	var v interface{}
	if err := json.Unmarshal([]byte(arguments), &v); err != nil {
		return "PARSE_ERROR"
	}
	return "OK"
}

// ─── B6: LLM API Key 防御 ─────────────────────────────────────

// ResolveAPIKey 解析 API Key——避免重复赋值。
// B6: 空字符串不覆盖已有 Key。新 Key 覆盖旧 Key。
func ResolveAPIKey(existing, newKey string) string {
	if newKey == "" {
		return existing // 空字符串不覆盖
	}
	return newKey
}

// ─── I8: Event layer 字段 ─────────────────────────────────────

// coreEvents 是所有 core 层事件。
var coreEvents = map[string]bool{
	"GoalCreated":               true,
	"MissionGenerated":          true,
	"ActionScheduled":           true,
	"ActionApproved":            true,
	"ActionCompleted":           true,
	"ActionFailed":              true,
	"GoalCompleted":             true,
	"GoalFailed":                true,
	"PipelinePaused":            true,
	"PipelineResumed":           true,
	"PlanRequested":             true,
	"ActionCancelled":           true,
	"HumanInterventionRequested": true,
	"PluginRegistered":          true,
	"PluginUnregistered":        true,
}

// IsCoreEvent 判断事件是否属于 core 层。
// I8: core→同步handler<1ms。side_effect→异步goroutine pool。
func IsCoreEvent(eventType string) bool {
	return coreEvents[eventType]
}

// ─── I9: IPC JSON 紧凑单行输出 ─────────────────────────────────

// ValidateCompactJSON 验证 JSON 是否为紧凑单行格式。
// I9: 子进程 json.Marshal 产物直接输出。daemon 读到非 JSON 行→忽略+告警。
func ValidateCompactJSON(line string) (bool, error) {
	// 必须不含真实换行符（紧凑单行）
	if strings.Contains(line, "\n") {
		return false, nil
	}
	// 必须可解析为 JSON
	var v interface{}
	if err := json.Unmarshal([]byte(line), &v); err != nil {
		return false, err
	}
	return true, nil
}
