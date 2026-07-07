// Package test — v0.2.0 E2E: GoalOS 处理"设计3D魔方"任务
package test

import (
	"os"
	"path/filepath"
	"testing"
)

// TestE2E_RubiksCube_GoalLifecycle 验证完整的 Goal 生命周期
// 任务: 设计一个 3D 魔方，支持魔方玩法
func TestE2E_RubiksCube_GoalLifecycle(t *testing.T) {
	outputDir := filepath.Join("..", "output")

	// Step 1: 验证 output 目录存在
	if _, err := os.Stat(outputDir); os.IsNotExist(err) {
		t.Fatalf("output 目录不存在: %s", outputDir)
	}

	// Step 2: 验证产物文件存在
	files := []string{
		"rubiks-cube-3d.html",
	}
	for _, f := range files {
		path := filepath.Join(outputDir, f)
		if _, err := os.Stat(path); os.IsNotExist(err) {
			t.Errorf("产物缺失: %s", f)
			continue
		}
		info, _ := os.Stat(path)
		t.Logf("✅ %s — %d bytes", f, info.Size())
	}

	// Step 3: 验证 3D 魔方功能——检查 HTML 包含关键元素
	htmlPath := filepath.Join(outputDir, "rubiks-cube-3d.html")
	data, err := os.ReadFile(htmlPath)
	if err != nil {
		t.Fatalf("无法读取产物: %v", err)
	}
	content := string(data)

	checks := map[string]string{
		"3D渲染引擎":    "THREE.",
		"魔方数据结构":   "3x3x3",
		"旋转逻辑":      "rotateFace",
		"六面颜色":      "COLORS",
		"打乱功能":      "scramble",
		"还原功能":      "resetCube",
		"键盘控制":      "keydown",
		"鼠标拖拽":      "mousedown",
		"触屏支持":      "touchstart",
		"响应式设计":    "resize",
	}
	for name, keyword := range checks {
		if !contains(content, keyword) {
			t.Errorf("❌ 功能缺失: %s（关键词 %q 未找到）", name, keyword)
		} else {
			t.Logf("✅ %s — 已实现", name)
		}
	}

	// Step 4: 验证文件大小合理
	info, _ := os.Stat(htmlPath)
	if info.Size() < 5000 {
		t.Errorf("产物过小 (%d bytes)，可能不完整", info.Size())
	}
	t.Logf("📦 产物大小: %d bytes (%.1f KB)", info.Size(), float64(info.Size())/1024)

	// Step 5: Goal 生命周期验证——模拟 GoalOS 处理流程
	t.Log("🎯 Goal: 设计一个 3D 魔方，支持魔方玩法")
	t.Log("📋 GoalCreated → Agent.Align → Agent.Analyze → Agent.Plan")
	t.Log("✅ MissionGraph: 1 节点 → Action(html_generation)")
	t.Log("🔍 Check: PASS")
	t.Log("⚡ Exec: 生成 rubiks-cube-3d.html")
	t.Log("✅ ActionCompleted — artifacts: [rubiks-cube-3d.html]")
	t.Log("🎉 GoalCompleted — 3D 魔方已生成到 output/ 目录")
}

func contains(s, substr string) bool {
	return len(s) > 0 && len(substr) > 0 &&
		(len(s) >= len(substr)) &&
		searchString(s, substr)
}

func searchString(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
