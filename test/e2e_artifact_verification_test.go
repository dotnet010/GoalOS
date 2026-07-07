// Package test — v0.2.0: GoalOS 产物验证 E2E
// 验证 GoalCompleted 之前必须确认产物真实可用
package test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestE2E_ArtifactVerification 验证产物质量——不只是文件存在
// GoalOS 流程缺陷发现: Three.js 加载顺序错误→页面空白。
// 修复: GoalCompleted 前增加依赖加载顺序验证。
func TestE2E_ArtifactVerification(t *testing.T) {
	outputDir := filepath.Join("..", "output")
	htmlPath := filepath.Join(outputDir, "rubiks-cube-3d.html")
	data, err := os.ReadFile(htmlPath)
	if err != nil {
		t.Fatalf("无法读取产物: %v", err)
	}
	content := string(data)

	// 1. 验证依赖库在业务代码之前加载
	threeJSPos := strings.Index(content, "three.min.js")
	threeUsagePos := strings.Index(content, "new THREE.Scene")
	if threeJSPos < 0 {
		t.Error("❌ 缺失 Three.js 依赖")
	} else if threeJSPos > threeUsagePos {
		t.Errorf("❌ 依赖加载顺序错误: Three.js (%d) 在 THREE.Scene 使用 (%d) 之后——页面无法渲染",
			threeJSPos, threeUsagePos)
	} else {
		t.Logf("✅ 依赖加载顺序正确: Three.js (%d) 在 THREE.Scene (%d) 之前", threeJSPos, threeUsagePos)
	}

	// 2. 验证 canvas 元素存在
	if !strings.Contains(content, `<canvas id="cube"`) {
		t.Error("❌ 缺失 canvas 渲染目标")
	} else {
		t.Log("✅ canvas 渲染目标存在")
	}

	// 3. 验证渲染循环存在
	if !strings.Contains(content, "requestAnimationFrame(render)") {
		t.Error("❌ 缺失渲染循环")
	} else {
		t.Log("✅ 渲染循环存在")
	}

	// 4. 验证无重复加载
	count := strings.Count(content, "three.min.js")
	if count > 1 {
		t.Errorf("❌ Three.js 重复加载 %d 次", count)
	} else {
		t.Log("✅ Three.js 单次加载")
	}

	t.Log("🎯 GoalOS 产物验证: 依赖顺序+渲染目标+渲染循环+无重复加载 = PASS")
}
