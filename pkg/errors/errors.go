// Package errors 定义 GoalOS 系统错误码体系。
// 分为三类：业务错误(Business)、系统错误(System)、安全错误(Security)。
// 所有 API 非 2xx 响应统一使用 ErrorResponse JSON 格式。
//
// 设计依据：R200。
package errors

import "fmt"

// Code 是系统错误码。
type Code string

// ─── 业务错误 (1xxx) ───
const (
	CodeGoalNotFound  Code = "GOAL_NOT_FOUND"  // Goal 不存在
	CodeInvalidRequest Code = "INVALID_REQUEST" // 请求参数无效
	CodeGoalNotRunning Code = "GOAL_NOT_RUNNING" // Goal 不在执行状态
	CodeGoalAlreadyCompleted Code = "GOAL_ALREADY_COMPLETED" // Goal 已完成
)

// ─── 系统错误 (2xxx) ───
const (
	CodeDaemonNotReady  Code = "DAEMON_NOT_READY"  // Daemon 未就绪
	CodeInternalError   Code = "INTERNAL_ERROR"    // 内部错误
	CodeTimeout         Code = "TIMEOUT"            // 操作超时
	CodeStorageError    Code = "STORAGE_ERROR"      // 存储错误
)

// ─── 安全错误 (3xxx) ───
const (
	CodeUnauthorized    Code = "UNAUTHORIZED"      // 未授权
	CodeForbidden       Code = "FORBIDDEN"         // 权限不足
	CodeTokenExpired    Code = "TOKEN_EXPIRED"     // Token 过期
	CodeTokenInvalid    Code = "TOKEN_INVALID"     // Token 无效
)

// APIError 是 API 错误响应结构体。
type APIError struct {
	Code    Code   `json:"code"`    // 错误码
	Message string `json:"message"` // 人类可读消息
}

// Error 实现 error 接口。
func (e *APIError) Error() string {
	return fmt.Sprintf("[%s] %s", e.Code, e.Message)
}

// New 创建一个 APIError。
func New(code Code, msg string) *APIError {
	return &APIError{Code: code, Message: msg}
}

// ErrorResponse 是标准 API 错误响应。
type ErrorResponse struct {
	Error APIError `json:"error"`
}

// NewResponse 创建一个标准错误响应。
func NewResponse(code Code, msg string) ErrorResponse {
	return ErrorResponse{Error: APIError{Code: code, Message: msg}}
}
