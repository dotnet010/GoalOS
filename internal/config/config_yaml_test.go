// Config YAML 解析测试 — 验证 gopkg.in/yaml.v3 解析。
package config_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/goalos/goalos/internal/config"
)

// TestLoadYAMLFile 验证从 YAML 文件加载配置覆盖默认值。
func TestLoadYAMLFile(t *testing.T) {
	dir := t.TempDir()
	yamlPath := filepath.Join(dir, "daemon.yaml")

	yamlContent := `
daemon:
  port: 18921
  autonomy_level: observe
  idle_timeout: 10m
llm:
  provider: openai
  model: gpt-4o
  max_tokens: 4096
persona: warm
`
	if err := os.WriteFile(yamlPath, []byte(yamlContent), 0644); err != nil {
		t.Fatal(err)
	}

	cfg, err := config.Load(yamlPath)
	if err != nil {
		t.Fatalf("Load failed: %v", err)
	}

	// 验证 YAML 值覆盖默认值
	if cfg.Daemon.Port != 18921 {
		t.Errorf("expected port=18921, got %d", cfg.Daemon.Port)
	}
	if cfg.Daemon.AutonomyLevel != "observe" {
		t.Errorf("expected autonomy=observe, got %s", cfg.Daemon.AutonomyLevel)
	}
	if cfg.LLM.Provider != "openai" {
		t.Errorf("expected provider=openai, got %s", cfg.LLM.Provider)
	}
	if cfg.LLM.Model != "gpt-4o" {
		t.Errorf("expected model=gpt-4o, got %s", cfg.LLM.Model)
	}
	if cfg.LLM.MaxTokens != 4096 {
		t.Errorf("expected max_tokens=4096, got %d", cfg.LLM.MaxTokens)
	}
	if cfg.Persona != "warm" {
		t.Errorf("expected persona=warm, got %s", cfg.Persona)
	}

	// 验证未覆盖的字段保持默认值
	if cfg.Daemon.ShutdownTimeout.String() != "5s" {
		t.Errorf("expected shutdown_timeout=5s (default), got %s", cfg.Daemon.ShutdownTimeout)
	}
}

// TestLoadYAMLFile_Partial 验证部分 YAML 文件仅覆盖指定字段。
func TestLoadYAMLFile_Partial(t *testing.T) {
	dir := t.TempDir()
	yamlPath := filepath.Join(dir, "daemon.yaml")

	yamlContent := `
daemon:
  port: 9999
`
	if err := os.WriteFile(yamlPath, []byte(yamlContent), 0644); err != nil {
		t.Fatal(err)
	}

	cfg, err := config.Load(yamlPath)
	if err != nil {
		t.Fatalf("Load failed: %v", err)
	}

	if cfg.Daemon.Port != 9999 {
		t.Errorf("expected port=9999, got %d", cfg.Daemon.Port)
	}
	// 其余应保持默认
	if cfg.Daemon.AutonomyLevel != "autonomous" {
		t.Errorf("autonomy should stay default=approve, got %s", cfg.Daemon.AutonomyLevel)
	}
	if cfg.LLM.Provider != "anthropic" {
		t.Errorf("provider should stay default=anthropic, got %s", cfg.LLM.Provider)
	}
}
