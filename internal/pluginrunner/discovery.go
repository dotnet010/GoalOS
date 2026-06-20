// Package pluginrunner — Plugin 发现与加载机制。
// 扫描 ~/.goalos/plugins/ 目录。读取 plugin.json manifest。验证签名。
// 发现结果缓存到内存。Plugin 按需启动。
//
// 设计依据：08 沙箱规范 §8、R197。
package pluginrunner

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// PluginManifest 是 plugin.json 的结构。
type PluginManifest struct {
	Name                  string   `json:"name"`
	Type                  string   `json:"type"` // "capability"|"agent"|"channel"
	Version               string   `json:"version"`
	Signature             string   `json:"signature"`              // SHA256
	Binary                string   `json:"binary"`                // 可执行文件路径（相对于 manifest 目录）
	DeclaredCapabilities  []string `json:"declared_capabilities"` // 声明提供的 Capability
	Description           string   `json:"description"`
}

// DiscoveredPlugin 是已发现的 Plugin。
type DiscoveredPlugin struct {
	Manifest   PluginManifest
	BinaryPath string // 可执行文件的绝对路径
	PluginDir  string // Plugin 目录
}

// Discover 扫描 pluginsDir 下所有子目录，读取 plugin.json，返回已发现的 Plugin 列表。
// 不验证签名（W5）。不启动子进程——仅发现。
func Discover(pluginsDir string) ([]DiscoveredPlugin, error) {
	var plugins []DiscoveredPlugin

	// 扫描 capability/、agent/、channel/ 三个子目录
	for _, typ := range []string{"capability", "agent", "channel"} {
		typeDir := filepath.Join(pluginsDir, typ)
		entries, err := os.ReadDir(typeDir)
		if err != nil {
			continue // 目录不存在——正常
		}

		for _, entry := range entries {
			if !entry.IsDir() {
				continue
			}
			pluginDir := filepath.Join(typeDir, entry.Name())
			manifestPath := filepath.Join(pluginDir, "plugin.json")

			data, err := os.ReadFile(manifestPath)
			if err != nil {
				continue // 缺少 manifest——跳过
			}

			var manifest PluginManifest
			if err := json.Unmarshal(data, &manifest); err != nil {
				continue // manifest 格式错误——跳过
			}

			// 验证必填字段
			if manifest.Name == "" || manifest.Type == "" || manifest.Binary == "" {
				continue
			}

			binaryPath := filepath.Join(pluginDir, manifest.Binary)
			if _, err := os.Stat(binaryPath); os.IsNotExist(err) {
				continue // 二进制文件不存在——跳过
			}

			plugins = append(plugins, DiscoveredPlugin{
				Manifest:   manifest,
				BinaryPath: binaryPath,
				PluginDir:  pluginDir,
			})
		}
	}

	return plugins, nil
}

// FindByCapability 在已发现的 Plugin 中查找提供指定 capability 的 Plugin。
func FindByCapability(plugins []DiscoveredPlugin, capability string) *DiscoveredPlugin {
	for _, p := range plugins {
		for _, c := range p.Manifest.DeclaredCapabilities {
			if c == capability {
				return &p
			}
		}
	}
	return nil
}

// FindByActionType 根据 Action 类型查找匹配的 Plugin。
// ActionType 格式为 "resource.action"（如 "shell.execute"）。
func FindByActionType(plugins []DiscoveredPlugin, actionType string) *DiscoveredPlugin {
	// 提取 resource 部分：shell.execute → shell
	parts := strings.SplitN(actionType, ".", 2)
	resource := parts[0]

	for _, p := range plugins {
		for _, c := range p.Manifest.DeclaredCapabilities {
			if strings.HasPrefix(c, resource+".") {
				return &p
			}
		}
	}
	return nil
}

// PluginDiscovery 是 Plugin 发现器的封装。
type PluginDiscovery struct {
	pluginsDir string
	cache      []DiscoveredPlugin
}

// NewPluginDiscovery 创建 Plugin 发现器。
func NewPluginDiscovery(pluginsDir string) *PluginDiscovery {
	return &PluginDiscovery{pluginsDir: pluginsDir}
}

// Refresh 重新扫描插件目录并更新缓存。
func (pd *PluginDiscovery) Refresh() error {
	plugins, err := Discover(pd.pluginsDir)
	if err != nil {
		return fmt.Errorf("discovery: 扫描失败: %w", err)
	}
	pd.cache = plugins
	return nil
}

// Find 在缓存中查找匹配 actionType 的 Plugin。
func (pd *PluginDiscovery) Find(actionType string) *DiscoveredPlugin {
	return FindByActionType(pd.cache, actionType)
}

// List 返回所有已发现的 Plugin。
func (pd *PluginDiscovery) List() []DiscoveredPlugin {
	return pd.cache
}
