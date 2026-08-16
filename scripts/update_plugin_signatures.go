// update_plugin_signatures.go — CI 插件签名更新工具（跨平台）。
//
// 动机: docker-publish 的 bash+jq 步骤在 Linux 可用；Windows 每日构建的
// PowerShell 等价实现静默失效（CI 红出——manifest 签名未更新，releasecheck
// signature mismatch）。统一为 go run 工具：两平台行为一致、可本地验证。
//
// 行为: 遍历 plugins/capability/*/plugin.json——binary 字段解析构建产物
// （Windows 补 .exe 后缀），以产物实际 SHA-256 回写 signature 字段
// （保留 manifest 其余字段）。产物缺失=跳过并输出提示。
package main

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
)

func main() {
	capDir := "plugins/capability"
	entries, err := os.ReadDir(capDir)
	if err != nil {
		fmt.Fprintf(os.Stderr, "update_plugin_signatures: %v\n", err)
		os.Exit(1)
	}
	updated := 0
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		manifestPath := filepath.Join(capDir, e.Name(), "plugin.json")
		data, err := os.ReadFile(manifestPath)
		if err != nil {
			continue
		}
		var m map[string]any
		if err := json.Unmarshal(data, &m); err != nil {
			fmt.Fprintf(os.Stderr, "update_plugin_signatures: %s 解析失败: %v\n", manifestPath, err)
			continue
		}
		binary, _ := m["binary"].(string)
		if binary == "" {
			continue
		}
		binPath := filepath.Join(capDir, e.Name(), binary)
		if runtime.GOOS == "windows" {
			binPath += ".exe" // manifest 保持跨平台名（无后缀）
		}
		binData, err := os.ReadFile(binPath)
		if err != nil {
			fmt.Printf("skip %s: binary not found: %s\n", manifestPath, binary)
			continue
		}
		m["signature"] = fmt.Sprintf("sha256:%x", sha256.Sum256(binData))
		out, err := json.MarshalIndent(m, "", "  ")
		if err != nil {
			fmt.Fprintf(os.Stderr, "update_plugin_signatures: %s 序列化失败: %v\n", manifestPath, err)
			continue
		}
		if err := os.WriteFile(manifestPath, out, 0644); err != nil {
			fmt.Fprintf(os.Stderr, "update_plugin_signatures: %s 写入失败: %v\n", manifestPath, err)
			continue
		}
		updated++
		fmt.Printf("Updated %s -> %s\n", manifestPath, m["signature"])
	}
	if updated == 0 {
		fmt.Println("update_plugin_signatures: 无插件签名更新（产物缺失或全部跳过）")
		os.Exit(1)
	}
}
