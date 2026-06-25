package llm

import (
	"errors"
	"fmt"

	openai "github.com/sashabaranov/go-openai"
)

// ClassifiedError 是分类后的 LLM 错误。
type ClassifiedError struct {
	Err        error  // 原始错误
	Retryable  bool   // 是否可重试
	StatusCode int    // HTTP 状态码
	Message    string // 人类可读错误描述
}

func (e *ClassifiedError) Error() string {
	return fmt.Sprintf("llm error [status=%d, retryable=%v]: %s", e.StatusCode, e.Retryable, e.Message)
}

func (e *ClassifiedError) Unwrap() error {
	return e.Err
}

// ClassifyLLMError 对 LLM API 错误进行分类。
// 规则：
//   - 401 (Unauthorized)：不可重试。API Key 无效或过期
//   - 429 (Rate Limited)：可重试。需要等待后重试
//   - 500+ (Server Error)：可重试。服务端临时故障
//   - 其他：不可重试
//
// 设计依据：go-openai 最佳实践 §Error Handling。
func ClassifyLLMError(err error) *ClassifiedError {
	var apiErr *openai.APIError
	if errors.As(err, &apiErr) {
		switch {
		case apiErr.HTTPStatusCode == 401:
			return &ClassifiedError{
				Err: err, Retryable: false, StatusCode: 401,
				Message: "API Key 无效或已过期，请检查认证配置",
			}
		case apiErr.HTTPStatusCode == 429:
			return &ClassifiedError{
				Err: err, Retryable: true, StatusCode: 429,
				Message: "API 速率限制，将在退避后重试",
			}
		case apiErr.HTTPStatusCode >= 500:
			return &ClassifiedError{
				Err: err, Retryable: true, StatusCode: apiErr.HTTPStatusCode,
				Message: fmt.Sprintf("服务端错误 (HTTP %d)，将重试", apiErr.HTTPStatusCode),
			}
		default:
			return &ClassifiedError{
				Err: err, Retryable: false, StatusCode: apiErr.HTTPStatusCode,
				Message: fmt.Sprintf("未处理的 API 错误 (HTTP %d)", apiErr.HTTPStatusCode),
			}
		}
	}
	// 非 API 错误（网络错误、超时等）— 可重试
	return &ClassifiedError{
		Err: err, Retryable: true, StatusCode: 0,
		Message: fmt.Sprintf("网络/系统错误: %v", err),
	}
}

// IsAuthError 判断是否为认证错误（不可重试）。
func IsAuthError(err error) bool {
	return errors.Is(err, &ClassifiedError{StatusCode: 401}) ||
		(ClassifyLLMError(err).StatusCode == 401)
}

// IsRateLimit 判断是否为速率限制错误。
func IsRateLimit(err error) bool {
	var apiErr *openai.APIError
	return errors.As(err, &apiErr) && apiErr.HTTPStatusCode == 429
}
