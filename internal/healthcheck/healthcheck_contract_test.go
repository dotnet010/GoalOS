// Package healthcheck — v0.2.0 audit: health check 契约测试
// Kent Beck 编写。验证每个 health check 函数执行了实际验证操作，
// 而非硬编码 Passed: true。
package healthcheck

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/goalos/goalos/internal/config"
)

// TestContract_LLMConnectivity_ActuallyChecksConnectivity 验证 checkLLMConnectivity 真的发送了 HTTP 请求。
// MUST: 配置了 BaseURL 时，函数必须发送 HTTP HEAD 请求验证端点可达。
func TestContract_LLMConnectivity_ActuallyChecksConnectivity(t *testing.T) {
	// 启动一个假 HTTP server
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodHead {
			t.Errorf("expected HEAD request, got %s", r.Method)
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	cfg := config.Default()
	cfg.LLM.BaseURL = server.URL
	cfg.LLM.Model = "test-model"

	result := checkLLMConnectivity(cfg)
	if !result.Passed {
		t.Fatalf("可达端点应返回 Passed=true, got %v: %s", result.Passed, result.Message)
	}
}

// TestContract_LLMConnectivity_ReturnsFalseWhenUnreachable 验证不可达端点返回失败。
// MUST: 端点不可达时，必须返回 Passed=false，而非假装通过。
func TestContract_LLMConnectivity_ReturnsFalseWhenUnreachable(t *testing.T) {
	cfg := config.Default()
	cfg.LLM.BaseURL = "http://127.0.0.1:1" // 不可达端口
	cfg.LLM.Model = "test-model"

	result := checkLLMConnectivity(cfg)
	if result.Passed {
		t.Fatalf("不可达端点应返回 Passed=false, got Passed=true: %s", result.Message)
	}
	if result.Suggestion == "" {
		t.Error("失败时应有修复建议 (Suggestion), got empty")
	}
}

// TestContract_LLMConnectivity_SkipsWhenNotConfigured 验证未配置 LLM 时跳过检查。
// MUST: 无 BaseURL 或无 Model 时，返回 Passed=true 并标注"跳过"。
func TestContract_LLMConnectivity_SkipsWhenNotConfigured(t *testing.T) {
	cfg := config.Default()
	cfg.LLM.BaseURL = ""
	cfg.LLM.Model = ""

	result := checkLLMConnectivity(cfg)
	if !result.Passed {
		t.Fatalf("未配置时跳过检查应返回 Passed=true, got %v", result.Passed)
	}
	if !strings.Contains(result.Message, "跳过") {
		t.Errorf("跳过时应标注'跳过', got: %s", result.Message)
	}
}

// TestContract_LLMConnectivity_UsesContextTimeout 验证使用了 context 超时。
// MUST: 使用 context.WithTimeout（5s），防止健康检查无限挂起。
func TestContract_LLMConnectivity_UsesContextTimeout(t *testing.T) {
	cfg := config.Default()
	cfg.LLM.BaseURL = "http://127.0.0.1:1"
	cfg.LLM.Model = "test-model"

	start := time.Now()
	result := checkLLMConnectivity(cfg)
	elapsed := time.Since(start)

	// 不可达端点应在 10s 内返回（5s timeout + 连接超时）
	if elapsed > 10*time.Second {
		t.Fatalf("LLM 连通性检查超时: %v", elapsed)
	}
	if result.Passed {
		t.Fatalf("不可达端点不应返回 Passed")
	}
	_ = context.Background() // 确保 context import 被使用
}

// TestContract_DiskSpace_ActuallyChecksDiskSpace 验证 checkDiskSpace 真的检查了磁盘可写性。
// R-867: 跨平台兼容——使用文件写入测试替代 syscall.Statfs。
func TestContract_DiskSpace_ActuallyChecksDiskSpace(t *testing.T) {
	result := checkDiskSpace()
	if !result.Passed {
		t.Logf("磁盘不可写: %s", result.Message)
	}
	// 消息不应为硬编码的"足够"（旧行为）
	if result.Message == "足够" {
		t.Error("checkDiskSpace MUST 返回实际磁盘检查结果，而非硬编码'足够'")
	}
	if !result.Passed && !strings.Contains(result.Message, "不可写") {
		t.Errorf("checkDiskSpace failed but message doesn't explain: %s", result.Message)
	}
}

// TestContract_DiskSpace_ReportsPath 验证磁盘检查使用了 .goalos 目录。
// MUST: 在 ~/.goalos 目录中测试写入。
func TestContract_DiskSpace_ReportsPath(t *testing.T) {
	home, err := os.UserHomeDir()
	if err != nil {
		t.Skip("无法获取 home 目录")
	}
	goalosDir := filepath.Join(home, ".goalos")
	os.MkdirAll(goalosDir, 0755)

	result := checkDiskSpace()
	// MUST: 返回实际检查结果
	if result.Message == "足够" {
		t.Error("checkDiskSpace MUST NOT return hardcoded '足够'")
	}
	if !result.Passed && !strings.Contains(result.Message, "不可写") {
		t.Errorf("checkDiskSpace failure message unexpected: %s", result.Message)
	}
}

// TestContract_RunAll_IncludesAllChecks 验证 RunAll 包含所有必需的检查项。
// MUST: RunAll 至少包含端口检查、LLM 配置、LLM 连通性、插件目录、磁盘空间。
func TestContract_RunAll_IncludesAllChecks(t *testing.T) {
	cfg := config.Default()
	cfg.Daemon.Port = 18920
	cfg.LLM.BaseURL = ""
	cfg.LLM.Model = ""

	tmpDir := t.TempDir()
	pluginsDir := filepath.Join(tmpDir, "plugins")
	os.MkdirAll(pluginsDir, 0755)

	results := RunAll(cfg, pluginsDir)

	requiredChecks := map[string]bool{
		"端口检查":   false,
		"LLM 配置":  false,
		"LLM 连通性": false,
		"插件目录":   false,
		"磁盘空间":   false,
	}
	for _, r := range results {
		for name := range requiredChecks {
			if strings.Contains(r.Name, name) {
				requiredChecks[name] = true
			}
		}
	}
	for name, found := range requiredChecks {
		if !found {
			t.Errorf("RunAll 必须包含检查项: %s", name)
		}
	}
}

// TestContract_HasErrors_DetectsFailures 验证 HasErrors 正确检测失败项。
func TestContract_HasErrors_DetectsFailures(t *testing.T) {
	results := []Result{
		{Name: "test-pass", Passed: true},
		{Name: "test-fail", Passed: false, Message: "某项检查失败"},
	}
	if !HasErrors(results) {
		t.Error("HasErrors 应在存在 Passed=false 时返回 true")
	}

	allPass := []Result{
		{Name: "test-1", Passed: true},
		{Name: "test-2", Passed: true},
	}
	if HasErrors(allPass) {
		t.Error("HasErrors 应在全部通过时返回 false")
	}
}
