// Package pluginrunner — Plugin 发现与加载机制。
// 扫描 ~/.goalos/plugins/ 目录。读取 plugin.json manifest。验证签名。
// 发现结果缓存到内存。Plugin 按需启动。
//
// v0.3.0 fix (M2): Ed25519 非对称签名替代 SHA256 hash。
// 设计依据：08 沙箱规范 §8.3、R-197。
package pluginrunner

import (
	"crypto/ed25519"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"
)

// PluginManifest 是 plugin.json 的结构。
type PluginManifest struct {
	Name                  string   `json:"name"`
	Type                  string   `json:"type"` // "capability"|"agent"|"channel"
	Version               string   `json:"version"`
	Signature             string   `json:"signature"`              // hex: Ed25519签名
	PublicKey             string   `json:"public_key,omitempty"`   // base64: Ed25519公钥（v0.3.0）
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

// Discover 扫描 pluginsDir 下所有子目录，读取 plugin.json，验证签名。
// v0.3.0: Ed25519 公钥签名验证优先，SHA256 兼容降级。
// 签名不匹配的 Plugin 被跳过并记录安全事件。不启动子进程——仅发现。
func Discover(pluginsDir string) ([]DiscoveredPlugin, error) {
	var plugins []DiscoveredPlugin

	for _, typ := range []string{"capability", "agent", "channel"} {
		typeDir := filepath.Join(pluginsDir, typ)
		entries, err := os.ReadDir(typeDir)
		if err != nil {
			continue
		}

		for _, entry := range entries {
			if !entry.IsDir() {
				continue
			}
			pluginDir := filepath.Join(typeDir, entry.Name())
			manifestPath := filepath.Join(pluginDir, "plugin.json")

			data, err := os.ReadFile(manifestPath)
			if err != nil {
				continue
			}

			var manifest PluginManifest
			if err := json.Unmarshal(data, &manifest); err != nil {
				continue
			}

			if manifest.Name == "" || manifest.Type == "" || manifest.Binary == "" {
				continue
			}

			binaryPath := filepath.Join(pluginDir, manifest.Binary)
			if _, err := os.Stat(binaryPath); os.IsNotExist(err) {
				continue
			}

			// v0.3.0: 签名验证——Ed25519 优先，SHA256 hash 兼容降级
			if manifest.Signature == "" {
				log.Printf("[PluginRunner] SECURITY: empty signature for %s — plugin rejected", manifest.Name)
				continue
			}
			if err := verifySignature(binaryPath, manifest.Signature, manifest.PublicKey); err != nil {
				log.Printf("[PluginRunner] SECURITY: signature verification failed for %s: %v", manifest.Name, err)
				continue
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

// verifySignature 验证 binary 文件的签名。
// v0.3.0 fix (M2): Ed25519 公钥签名优先（非对称、不可伪造）。
// publicKey 为空时降级到 SHA256 hash 验证（向后兼容旧 Plugin）。
func verifySignature(binaryPath, expected string, publicKey string) error {
	data, err := os.ReadFile(binaryPath)
	if err != nil {
		return fmt.Errorf("read binary: %w", err)
	}

	// Ed25519: 公钥签名验证
	if publicKey != "" {
		sigHex := strings.TrimPrefix(expected, "ed25519:")
		sig, sigErr := hex.DecodeString(sigHex)
		pub, pubErr := base64.RawURLEncoding.DecodeString(publicKey)
		if sigErr == nil && pubErr == nil && len(pub) == ed25519.PublicKeySize {
			if ed25519.Verify(pub, data, sig) {
				return nil
			}
			return fmt.Errorf("ed25519 signature mismatch")
		}
	}

	// SHA256 兼容降级：对旧 Plugin 仍支持 hash 验证
	if strings.HasPrefix(expected, "sha256:") {
		hash := sha256.Sum256(data)
		actual := "sha256:" + hex.EncodeToString(hash[:])
		if actual != expected {
			return fmt.Errorf("sha256 hash mismatch: expected %s", expected)
		}
		return nil
	}

	return fmt.Errorf("unsupported signature format: must be ed25519:<hex> or sha256:<hex>")
}

// GenerateEd25519KeyPair 生成 Ed25519 密钥对（供 Plugin 开发者使用）。
// 返回 (hex编码私钥, base64编码公钥)。
func GenerateEd25519KeyPair() (string, string, error) {
	pub, priv, err := ed25519.GenerateKey(nil)
	if err != nil {
		return "", "", fmt.Errorf("ed25519 keygen: %w", err)
	}
	return hex.EncodeToString(priv), base64.RawURLEncoding.EncodeToString(pub), nil
}

// SignWithEd25519 对二进制数据签名（供 Plugin 开发者使用）。
func SignWithEd25519(privKeyHex string, data []byte) (string, error) {
	privBytes, err := hex.DecodeString(privKeyHex)
	if err != nil {
		return "", fmt.Errorf("decode private key: %w", err)
	}
	if len(privBytes) != ed25519.PrivateKeySize {
		return "", fmt.Errorf("invalid Ed25519 private key size: %d", len(privBytes))
	}
	sig := ed25519.Sign(privBytes, data)
	return "ed25519:" + hex.EncodeToString(sig), nil
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
