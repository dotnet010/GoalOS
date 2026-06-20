package pluginrunner_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/goalos/goalos/internal/pluginrunner"
)

func TestDiscover(t *testing.T) {
	dir := t.TempDir()

	// 创建 capability/shell/ 目录和 plugin.json
	pluginDir := filepath.Join(dir, "capability", "shell")
	os.MkdirAll(pluginDir, 0755)

	manifest := `{
		"name": "shell-executor",
		"type": "capability",
		"version": "1.0.0",
		"signature": "sha256:abc123",
		"binary": "./shell-executor",
		"declared_capabilities": ["shell.execute"],
		"description": "Shell 命令执行器"
	}`
	os.WriteFile(filepath.Join(pluginDir, "plugin.json"), []byte(manifest), 0644)
	// 创建虚拟 binary
	os.WriteFile(filepath.Join(pluginDir, "shell-executor"), []byte("fake"), 0755)

	plugins, err := pluginrunner.Discover(dir)
	if err != nil {
		t.Fatalf("Discover failed: %v", err)
	}
	if len(plugins) != 1 {
		t.Fatalf("expected 1 plugin, got %d", len(plugins))
	}
	if plugins[0].Manifest.Name != "shell-executor" {
		t.Errorf("expected shell-executor, got %s", plugins[0].Manifest.Name)
	}
	if plugins[0].Manifest.Type != "capability" {
		t.Errorf("expected capability, got %s", plugins[0].Manifest.Type)
	}
}

func TestFindByActionType(t *testing.T) {
	plugins := []pluginrunner.DiscoveredPlugin{
		{
			Manifest: pluginrunner.PluginManifest{
				Name:                 "shell-executor",
				Type:                 "capability",
				DeclaredCapabilities: []string{"shell.execute", "shell.read"},
			},
		},
		{
			Manifest: pluginrunner.PluginManifest{
				Name:                 "browser-executor",
				Type:                 "capability",
				DeclaredCapabilities: []string{"browser.click", "browser.open"},
			},
		},
	}

	found := pluginrunner.FindByActionType(plugins, "shell.execute")
	if found == nil {
		t.Fatal("expected to find shell-executor for shell.execute")
	}
	if found.Manifest.Name != "shell-executor" {
		t.Errorf("expected shell-executor, got %s", found.Manifest.Name)
	}

	found = pluginrunner.FindByActionType(plugins, "browser.click")
	if found == nil {
		t.Fatal("expected to find browser-executor for browser.click")
	}

	notFound := pluginrunner.FindByActionType(plugins, "database.delete")
	if notFound != nil {
		t.Errorf("expected nil for database.delete, got %s", notFound.Manifest.Name)
	}
}

func TestEmptyDir(t *testing.T) {
	dir := t.TempDir()
	plugins, err := pluginrunner.Discover(dir)
	if err != nil {
		t.Fatalf("Discover failed: %v", err)
	}
	if len(plugins) != 0 {
		t.Errorf("expected 0 plugins, got %d", len(plugins))
	}
}
