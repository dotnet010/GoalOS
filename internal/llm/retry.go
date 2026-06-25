package llm

import (
	"context"
	"fmt"
	"math"
	"time"
)

// RetryConfig 定义重试策略参数。
type RetryConfig struct {
	MaxAttempts int           // 最大尝试次数（含首次）。默认 3
	BaseDelay   time.Duration // 初始退避延迟。默认 1s
	MaxDelay    time.Duration // 最大退避延迟上限。默认 30s
}

// DefaultRetryConfig 返回默认重试配置。
func DefaultRetryConfig() RetryConfig {
	return RetryConfig{
		MaxAttempts: 3,
		BaseDelay:   1 * time.Second,
		MaxDelay:    30 * time.Second,
	}
}

// isRetryableStatus 判断 HTTP 状态码是否可重试。
func isRetryableStatus(code int) bool {
	return code == 429 || code >= 500
}

// WithRetry 执行带指数退避重试的 LLM 调用。
// fn 每次被调用时传入当前尝试次数（从 1 开始）。
// 仅在以下情况重试：
//   - 网络/系统错误（非 API 错误）
//   - HTTP 429（速率限制）
//   - HTTP 5xx（服务端错误）
//
// 401 等认证错误立即返回，不重试。
//
// 设计依据：go-openai 最佳实践 §Error Handling, §Rate Limiting。
func WithRetry[T any](ctx context.Context, cfg RetryConfig, fn func(attempt int) (T, error)) (T, error) {
	var zero T
	var lastErr error

	for attempt := 1; attempt <= cfg.MaxAttempts; attempt++ {
		// 检查 context 是否已取消
		select {
		case <-ctx.Done():
			return zero, fmt.Errorf("重试被取消: %w", ctx.Err())
		default:
		}

		result, err := fn(attempt)
		if err == nil {
			return result, nil
		}
		lastErr = err

		// 分类错误，判断是否可重试
		classified := ClassifyLLMError(err)

		// 认证错误不重试
		if classified.StatusCode == 401 {
			return zero, classified
		}

		// 不可重试且非网络错误→直接返回
		if !classified.Retryable && classified.StatusCode != 0 {
			return zero, classified
		}

		// 最后一次尝试不等待
		if attempt == cfg.MaxAttempts {
			break
		}

		// 计算退避延迟：baseDelay * 2^(attempt-1)，上限 maxDelay
		delay := time.Duration(math.Min(
			float64(cfg.BaseDelay)*math.Pow(2, float64(attempt-1)),
			float64(cfg.MaxDelay),
		))

		// 速率限制 (429) 额外等待
		if isRetryableStatus(classified.StatusCode) {
			delay = time.Duration(math.Max(float64(delay), 2*float64(time.Second)))
		}

		select {
		case <-ctx.Done():
			return zero, fmt.Errorf("重试等待被取消: %w", ctx.Err())
		case <-time.After(delay):
		}
	}

	return zero, fmt.Errorf("重试 %d 次后仍失败: %w", cfg.MaxAttempts, lastErr)
}
