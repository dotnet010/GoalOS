// Package errorcategory — 框架基础设施 F2（Week 0）
// CategorizedError 接口 + 4 种 ErrorCategory 枚举。
// PipelineRunner 读取 Category()→自动路由 Decide 路径。
// R-771: ErrorTemporary→PipelineRunner 内部触发新 Session 重做。
// R-795: MaxTokenTTL 全局硬上限 24h。
// CI 强制: check-error-swallow.sh 检测吞 error。

package errorcategory

import (
	"fmt"
	"time"
)

// MaxTokenTTL Capability Token 全局硬上限（R-795 — 会议 #122）。
// 无论 Action timeout 多大，Token 过期时间不得超过 24 小时。
const MaxTokenTTL = 24 * time.Hour

// ErrorCategory 错误分类。固定 4 值——不再增加（Linus：R-738）。
type ErrorCategory int

const (
	// ErrorTemporary 可重试。LLM 5xx、网络超时、资源临时不可用。
	// R-771: PipelineRunner 内部触发新 Session 重做 1 次。成功→CONTINUE，失败→ESCALATE。
	ErrorTemporary ErrorCategory = iota

	// ErrorPermanent 需人工介入。参数非法、权限拒绝、配置错误。直接 ESCALATE。
	ErrorPermanent

	// ErrorSecurity 安全违规。HMAC 失败、seccomp 违规、策略拒绝。
	// 发布 InvariantViolated + ESCALATE。
	ErrorSecurity

	// ErrorFatal 系统级错误。OOM、panic、进程崩溃。发布 GoalFailed。
	ErrorFatal
)

// CategorizedError 框架级错误接口。模块返回的 error 实现此接口→PipelineRunner 自动路由。
type CategorizedError interface {
	error
	Category() ErrorCategory
	Suggestion() string // 用户可见建议→映射到 failHints（R-748）
}

// Sentinel errors for errors.Is() matching.
var (
	ErrTemporary = &categorizedError{category: ErrorTemporary}
	ErrPermanent  = &categorizedError{category: ErrorPermanent}
	ErrSecurity   = &categorizedError{category: ErrorSecurity}
	ErrFatal      = &categorizedError{category: ErrorFatal}
)

type categorizedError struct {
	msg        string
	category   ErrorCategory
	suggestion string
	inner      error
}

func (e *categorizedError) Error() string { return e.msg }
func (e *categorizedError) Category() ErrorCategory { return e.category }
func (e *categorizedError) Suggestion() string { return e.suggestion }
func (e *categorizedError) Unwrap() error { return e.inner }
func (e *categorizedError) Is(target error) bool {
	if ce, ok := target.(*categorizedError); ok {
		return e.category == ce.category
	}
	return false
}

// NewTemporary 创建可重试错误。
func NewTemporary(msg, suggestion string) error {
	return &categorizedError{msg: msg, category: ErrorTemporary, suggestion: suggestion}
}

// NewPermanent 创建需人工介入错误。
func NewPermanent(msg, suggestion string) error {
	return &categorizedError{msg: msg, category: ErrorPermanent, suggestion: suggestion}
}

// NewSecurity 创建安全违规错误。
func NewSecurity(msg, suggestion string) error {
	return &categorizedError{msg: msg, category: ErrorSecurity, suggestion: suggestion}
}

// NewFatal 创建系统级错误。
func NewFatal(msg, suggestion string) error {
	return &categorizedError{msg: msg, category: ErrorFatal, suggestion: suggestion}
}

// Wrap 包装已有 error 并附加 Category。
func Wrap(err error, category ErrorCategory, msg, suggestion string) error {
	return &categorizedError{
		msg:        fmt.Sprintf("%s: %v", msg, err),
		category:   category,
		suggestion: suggestion,
		inner:      err,
	}
}

// CategoryOf 提取 error 的 ErrorCategory。非 CategorizedError 返回 -1。
func CategoryOf(err error) ErrorCategory {
	if ce, ok := err.(CategorizedError); ok {
		return ce.Category()
	}
	return -1
}
