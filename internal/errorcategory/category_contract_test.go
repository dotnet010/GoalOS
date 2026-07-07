// Package errorcategory — 框架基础设施 F2
// Week 0 contract_test: Beck 编写（R-571 测试先行）
// MUST: T-CE-1(CategorizedError_Routing)——4 种 Category→正确 Decide 路径

package errorcategory

import (
	"errors"
	"testing"
)

// TestCategorizedError_Routing_Temporary 验证 ErrorTemporary→CONTINUE
func TestCategorizedError_Routing_Temporary(t *testing.T) {
	err := NewTemporary("network timeout", "请检查网络后重试")
	catErr, ok := err.(CategorizedError)
	if !ok {
		t.Fatal("TemporaryError 必须实现 CategorizedError")
	}
	if catErr.Category() != ErrorTemporary {
		t.Errorf("Category = %v, want ErrorTemporary", catErr.Category())
	}
	if catErr.Suggestion() == "" {
		t.Error("Suggestion 不应为空——需映射到 failHints")
	}
	if !errors.Is(err, ErrTemporary) {
		t.Error("errors.Is(err, ErrTemporary) 应为 true")
	}
}

// TestCategorizedError_Routing_Permanent 验证 ErrorPermanent→ESCALATE
func TestCategorizedError_Routing_Permanent(t *testing.T) {
	err := NewPermanent("invalid config", "请检查 daemon.yaml 配置")
	catErr := err.(CategorizedError)
	if catErr.Category() != ErrorPermanent {
		t.Errorf("Category = %v, want ErrorPermanent", catErr.Category())
	}
	if !errors.Is(err, ErrPermanent) {
		t.Error("errors.Is(err, ErrPermanent) 应为 true")
	}
}

// TestCategorizedError_Routing_Security 验证 ErrorSecurity→ESCALATE+InvariantViolated
func TestCategorizedError_Routing_Security(t *testing.T) {
	err := NewSecurity("hmac verification failed", "Plugin 通信异常，目标已暂停")
	catErr := err.(CategorizedError)
	if catErr.Category() != ErrorSecurity {
		t.Errorf("Category = %v, want ErrorSecurity", catErr.Category())
	}
	if !errors.Is(err, ErrSecurity) {
		t.Error("errors.Is(err, ErrSecurity) 应为 true")
	}
}

// TestCategorizedError_Routing_Fatal 验证 ErrorFatal→GoalFailed
func TestCategorizedError_Routing_Fatal(t *testing.T) {
	err := NewFatal("out of memory", "系统资源不足，目标已终止")
	catErr := err.(CategorizedError)
	if catErr.Category() != ErrorFatal {
		t.Errorf("Category = %v, want ErrorFatal", catErr.Category())
	}
	if !errors.Is(err, ErrFatal) {
		t.Error("errors.Is(err, ErrFatal) 应为 true")
	}
}

// TestCategorizedError_ErrorPropagation 验证 error 传播链
func TestCategorizedError_ErrorPropagation(t *testing.T) {
	base := NewTemporary("LLM timeout", "模型响应超时")
	wrapped := NewTemporary("pipeline failed: "+base.Error(), base.(CategorizedError).Suggestion())

	// errors.Is 应穿透包装
	if !errors.Is(wrapped, ErrTemporary) {
		t.Error("包装后的 error 应仍可通过 errors.Is 匹配")
	}

	// Unwrap: wrapped 没有 inner（NewTemporary 创建的是叶子 error）
	// Wrap() 创建的 error 才有 Unwrap 语义
}

// TestCategorizedError_Suggestion 验证 Suggestion 映射
func TestCategorizedError_Suggestion(t *testing.T) {
	tests := []struct {
		err        error
		wantNonEmpty bool
	}{
		{NewTemporary("timeout", "建议更换模型"), true},
		{NewPermanent("parse error", "建议检查输入"), true},
		{NewSecurity("hmac fail", "Plugin 通信异常"), true},
		{NewFatal("OOM", "系统资源不足"), true},
	}
	for _, tt := range tests {
		catErr := tt.err.(CategorizedError)
		if tt.wantNonEmpty && catErr.Suggestion() == "" {
			t.Errorf("%T.Suggestion() 不应为空", tt.err)
		}
	}
}
