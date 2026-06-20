// Package config 实现 GoalOS 配置系统。
// 优先级：环境变量 > daemon.yaml > 默认值。
// 修改 daemon.yaml 后发送 SIGHUP 热加载。
//
// 设计依据：05 架构文档 §10、附录 B.6、R176、R203。
package config

import (
	"fmt"
	"os"
	"time"

	"gopkg.in/yaml.v3"
)

// Config 是 GoalOS 完整配置。
type Config struct {
	Daemon   DaemonConfig   `yaml:"daemon"`
	LLM      LLMConfig      `yaml:"llm"`
	Persona  string         `yaml:"persona"`  // "concise"|"warm"|"minimal"
}

// DaemonConfig 是 Daemon 运行时配置。
type DaemonConfig struct {
	Port            int           `yaml:"port"`             // HTTP 端口。默认 18920
	AutonomyLevel   string        `yaml:"autonomy_level"`  // "observe"|"suggest"|"approve"|"autonomous"。默认 "approve"
	IdleTimeout     time.Duration `yaml:"idle_timeout"`    // 空闲超时后退出。默认 5m
	ShutdownTimeout time.Duration `yaml:"shutdown_timeout"` // 优雅关闭超时。默认 5s
}

// LLMConfig 是 LLM Provider 配置。
type LLMConfig struct {
	Provider  string `yaml:"provider"`   // "anthropic"|"openai"|"ollama"。默认 "anthropic"
	Model     string `yaml:"model"`      // 模型名
	APIKeyEnv string `yaml:"api_key_env"` // API Key 环境变量名。默认 "ANTHROPIC_API_KEY"
	MaxTokens int    `yaml:"max_tokens"` // 最大 Token 数。默认 8192
	Timeout   time.Duration `yaml:"timeout"` // 请求超时。默认 120s
}

// Default 返回默认配置。
func Default() *Config {
	return &Config{
		Daemon: DaemonConfig{
			Port:            18920,
			AutonomyLevel:   "approve",
			IdleTimeout:     5 * time.Minute,
			ShutdownTimeout: 5 * time.Second,
		},
		LLM: LLMConfig{
			Provider:  "anthropic",
			Model:     "claude-sonnet-4-6",
			APIKeyEnv: "ANTHROPIC_API_KEY",
			MaxTokens: 8192,
			Timeout:   120 * time.Second,
		},
		Persona: "concise",
	}
}

// Load 加载配置。优先级：环境变量 > 文件 > 默认值。
func Load(path string) (*Config, error) {
	cfg := Default()

	// 尝试读取配置文件
	if path != "" {
		if _, err := os.Stat(path); err == nil {
			if err := loadYAML(path, cfg); err != nil {
				return nil, fmt.Errorf("config: 加载 %s 失败: %w", path, err)
			}
		}
	}

	// 环境变量覆盖
	applyEnv(cfg)

	return cfg, nil
}

// applyEnv 从环境变量覆盖配置。
func applyEnv(cfg *Config) {
	if v := os.Getenv("GOALOS_PORT"); v != "" {
		fmt.Sscanf(v, "%d", &cfg.Daemon.Port)
	}
	if v := os.Getenv("GOALOS_AUTONOMY"); v != "" {
		cfg.Daemon.AutonomyLevel = v
	}
	if v := os.Getenv("GOALOS_PERSONA"); v != "" {
		cfg.Persona = v
	}
	if v := os.Getenv("GOALOS_LLM_PROVIDER"); v != "" {
		cfg.LLM.Provider = v
	}
	if v := os.Getenv("GOALOS_LLM_MODEL"); v != "" {
		cfg.LLM.Model = v
	}
}

// loadYAML 从 YAML 文件加载配置，将文件值合并到 cfg 上（文件值覆盖默认值）。
func loadYAML(path string, cfg *Config) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	// 解析到临时结构，防止零值覆盖 cfg 已有默认值
	var fileCfg Config
	if err := yaml.Unmarshal(data, &fileCfg); err != nil {
		return fmt.Errorf("yaml parse: %w", err)
	}
	// 仅覆盖非零值字段
	if fileCfg.Daemon.Port != 0 {
		cfg.Daemon.Port = fileCfg.Daemon.Port
	}
	if fileCfg.Daemon.AutonomyLevel != "" {
		cfg.Daemon.AutonomyLevel = fileCfg.Daemon.AutonomyLevel
	}
	if fileCfg.Daemon.IdleTimeout != 0 {
		cfg.Daemon.IdleTimeout = fileCfg.Daemon.IdleTimeout
	}
	if fileCfg.Daemon.ShutdownTimeout != 0 {
		cfg.Daemon.ShutdownTimeout = fileCfg.Daemon.ShutdownTimeout
	}
	if fileCfg.LLM.Provider != "" {
		cfg.LLM.Provider = fileCfg.LLM.Provider
	}
	if fileCfg.LLM.Model != "" {
		cfg.LLM.Model = fileCfg.LLM.Model
	}
	if fileCfg.LLM.APIKeyEnv != "" {
		cfg.LLM.APIKeyEnv = fileCfg.LLM.APIKeyEnv
	}
	if fileCfg.LLM.MaxTokens != 0 {
		cfg.LLM.MaxTokens = fileCfg.LLM.MaxTokens
	}
	if fileCfg.LLM.Timeout != 0 {
		cfg.LLM.Timeout = fileCfg.LLM.Timeout
	}
	if fileCfg.Persona != "" {
		cfg.Persona = fileCfg.Persona
	}
	return nil
}
