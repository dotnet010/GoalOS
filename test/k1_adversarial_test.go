// Package test — v0.2.0 Week 5-6 K1: 对抗性欺骗测试
// 验证 FlowComposer 能检测 LLM 尝试绕过检查
// R-818: 检测函数已移至 FlowComposer——测试调用真实方法
package test

import (
	"testing"

	"github.com/goalos/goalos/internal/scheduler"
)

func TestK1_AdversarialDeception(t *testing.T) {
	fc := scheduler.NewFlowComposer(scheduler.NewFlowRegistry())

	t.Run("空操作检测_echo_skip", func(t *testing.T) {
		if fc.DetectDeceptivePattern("shell.execute", "echo skip") {
			t.Log("✅ 检测到 echo skip 欺骗模式")
		} else {
			t.Error("❌ 未检测到 echo skip 欺骗模式")
		}
	})

	t.Run("空操作检测_sleep", func(t *testing.T) {
		if fc.DetectDeceptivePattern("shell.execute", "sleep 0") {
			t.Log("✅ 检测到 sleep 欺骗模式")
		} else {
			t.Error("❌ 未检测到 sleep 欺骗模式")
		}
	})

	t.Run("合法命令_通过检测", func(t *testing.T) {
		if !fc.DetectDeceptivePattern("shell.execute", "npm install") {
			t.Log("✅ 合法命令通过检测")
		} else {
			t.Error("❌ 合法命令被误判为欺骗")
		}
	})

	t.Run("action_type不匹配", func(t *testing.T) {
		if fc.DetectMismatchedAction("code_review", "shell.execute") {
			t.Log("✅ 检测到 action_type 与 stage 不匹配")
		} else {
			t.Error("❌ 未检测到 action_type 不匹配")
		}
	})

	t.Run("合法匹配_通过检测", func(t *testing.T) {
		if !fc.DetectMismatchedAction("code_generation", "shell.execute") {
			t.Log("✅ 合法 stage-action 匹配通过")
		} else {
			t.Error("❌ 合法匹配被误判")
		}
	})

	t.Run("空参数列表_检测", func(t *testing.T) {
		if fc.DetectEmptyParams(map[string]interface{}{}) {
			t.Log("✅ 检测到空参数欺骗")
		} else {
			t.Error("❌ 未检测到空参数欺骗")
		}
	})
}
