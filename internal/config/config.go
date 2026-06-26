// Package config 实现 GoalOS 配置系统。
// 优先级：环境变量 > daemon.yaml > 默认值。
// 修改 daemon.yaml 后发送 SIGHUP 热加载。
//
// 设计依据：05 架构文档 §10、附录 B.6、R176、R203。
package config

import (
	"fmt"
	"os"
	"strings"
	"time"

	"gopkg.in/yaml.v3"
)

// Config 是 GoalOS 完整配置。
type Config struct {
	Daemon   DaemonConfig   `yaml:"daemon"`
	LLM      LLMConfig      `yaml:"llm"`
	MultiLLM MultiLLMConfig `yaml:"multi_llm"`
	Policy   PolicyConfig   `yaml:"policy"`
	Persona  string         `yaml:"persona"`  // "concise"|"warm"|"minimal"
}

// DaemonConfig 是 Daemon 运行时配置。
type DaemonConfig struct {
	Port            int           `yaml:"port"`             // HTTP 端口。默认 18920
	AutonomyLevel   string        `yaml:"autonomy_level"`  // "observe"|"suggest"|"approve"|"autonomous"。默认 "approve"
	IdleTimeout     time.Duration `yaml:"idle_timeout"`    // 空闲超时后退出。默认 5m
	ShutdownTimeout time.Duration `yaml:"shutdown_timeout"` // 优雅关闭超时。默认 5s
}



// MultiLLMConfig 是多模型验证配置（v1.1.0）。
type MultiLLMConfig struct {
	Enabled   bool               `yaml:"enabled"`
	Providers []MultiLLMProvider `yaml:"providers"`
}
// MultiLLMProvider 是多模型验证的单个 Provider 配置。
type MultiLLMProvider struct {
	Name       string   `yaml:"name"`
	Model      string   `yaml:"model"`
	APIKey     string   `yaml:"api_key"`
	BaseURL    string   `yaml:"base_url"`
	AllowedFor []string `yaml:"allowed_for"`
}

// PolicyConfig 是运行时策略配置（v1.1.0）。
type PolicyConfig struct {
	ApprovalTimeout       int     `yaml:"approval_timeout"`        // 审批超时秒数。默认 300
	TokenBudget           int     `yaml:"token_budget"`            // 单 Goal Token 上限。默认 1_000_000
	TokenWarning          float64 `yaml:"token_warning"`           // 预算警告阈值。默认 0.8
	AutoFixMax            int     `yaml:"auto_fix_max"`            // 自修正最大次数。默认 3
	FlowDegradeThreshold  int     `yaml:"flow_degrade_threshold"`  // Flow 降级触发次数。默认 3
	GoalAnchorInterval    int     `yaml:"goal_anchor_interval"`    // GoalAnchor 检查间隔。默认 20
	RecoveryRetryMax      int     `yaml:"recovery_retry_max"`      // Recovery 重试最大次数。默认 3
}

// LLMConfig 是 LLM Provider 配置。
type LLMConfig struct {
	Provider      string        `yaml:"provider"`        // "anthropic"|"openai"|"ollama"。默认 "anthropic"
	Model         string        `yaml:"model"`           // 模型名
	APIKeyEnv     string        `yaml:"api_key_env"`     // API Key 环境变量名。默认 "ANTHROPIC_API_KEY"
	APIKey        string        `yaml:"api_key"`         // v1.1.0: 直接配置 API Key（优先级低于环境变量）
	BaseURL       string        `yaml:"base_url"`        // API 基础 URL。Cloud 和 Ollama 均可配置
	MaxTokens     int           `yaml:"max_tokens"`      // 最大 Token 数。默认 8192
	Temperature   float32       `yaml:"temperature"`     // LLM 温度参数。0~2，默认 0.3
	Timeout       time.Duration `yaml:"timeout"`         // 请求超时。默认 120s
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
			Provider:    "anthropic",
			Model:       "claude-sonnet-4-6",
			APIKeyEnv:   "ANTHROPIC_API_KEY",
			BaseURL:     "", // 空表示使用默认 API 端点
			MaxTokens:   8192,
			Temperature: 0.3,
			Timeout:     120 * time.Second,
		},
			Policy: PolicyConfig{
				ApprovalTimeout: 300, TokenBudget: 1_000_000, TokenWarning: 0.8,
				AutoFixMax: 3, FlowDegradeThreshold: 3, GoalAnchorInterval: 20, RecoveryRetryMax: 3,
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
	if v := os.Getenv("GOALOS_LLM_BASE_URL"); v != "" {
		cfg.LLM.BaseURL = v
	}
	if v := os.Getenv("OLLAMA_BASE_URL"); v != "" {
		cfg.LLM.BaseURL = v // Ollama 环境变量覆盖
	}
	if v := os.Getenv("GOALOS_LLM_TEMPERATURE"); v != "" {
		var t float64
		if _, err := fmt.Sscanf(v, "%f", &t); err == nil {
			cfg.LLM.Temperature = float32(t)
		}
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
	if fileCfg.LLM.APIKey != "" {
		cfg.LLM.APIKey = fileCfg.LLM.APIKey
	}
	if fileCfg.LLM.MaxTokens != 0 {
		cfg.LLM.MaxTokens = fileCfg.LLM.MaxTokens
	}
	if fileCfg.LLM.Temperature != 0 {
		cfg.LLM.Temperature = fileCfg.LLM.Temperature
	}
	if fileCfg.LLM.BaseURL != "" {
		cfg.LLM.BaseURL = fileCfg.LLM.BaseURL
	}
	if fileCfg.LLM.Timeout != 0 {
		cfg.LLM.Timeout = fileCfg.LLM.Timeout
	if fileCfg.Policy.ApprovalTimeout != 0 { cfg.Policy.ApprovalTimeout = fileCfg.Policy.ApprovalTimeout }
	if fileCfg.Policy.TokenBudget != 0 { cfg.Policy.TokenBudget = fileCfg.Policy.TokenBudget }
	if fileCfg.Policy.TokenWarning != 0 { cfg.Policy.TokenWarning = fileCfg.Policy.TokenWarning }
	if fileCfg.Policy.AutoFixMax != 0 { cfg.Policy.AutoFixMax = fileCfg.Policy.AutoFixMax }
	if fileCfg.Policy.FlowDegradeThreshold != 0 { cfg.Policy.FlowDegradeThreshold = fileCfg.Policy.FlowDegradeThreshold }
	if fileCfg.Policy.GoalAnchorInterval != 0 { cfg.Policy.GoalAnchorInterval = fileCfg.Policy.GoalAnchorInterval }
	if fileCfg.Policy.RecoveryRetryMax != 0 { cfg.Policy.RecoveryRetryMax = fileCfg.Policy.RecoveryRetryMax }
	}
	if fileCfg.Persona != "" {
		cfg.Persona = fileCfg.Persona
	}
	return nil
}

// Reload 热加载配置——不重启 daemon（v1.1.0 UX1）。
func (cfg *Config) Reload(path string) error {
	if path == "" {
		return fmt.Errorf("config: no path for reload")
	}
	if _, err := os.Stat(path); os.IsNotExist(err) {
		return fmt.Errorf("config: %s not found", path)
	}
	oldPort := cfg.Daemon.Port
	if err := loadYAML(path, cfg); err != nil {
		return fmt.Errorf("config: reload failed: %w", err)
	}
	cfg.Daemon.Port = oldPort
	applyEnv(cfg)
	return nil
}

// Validate 校验配置合法性。不合法拒绝启动，给出具体错误信息（v1.1.0）。
func (cfg *Config) Validate() error {
	if cfg.Daemon.Port < 1 || cfg.Daemon.Port > 65535 {
		return fmt.Errorf("daemon.port 必须在 1-65535，当前: %d", cfg.Daemon.Port)
	}
	validAutonomy := map[string]bool{"observe":true,"suggest":true,"approve":true,"autonomous":true}
	if !validAutonomy[cfg.Daemon.AutonomyLevel] {
		return fmt.Errorf("daemon.autonomy_level 无效值 '%s'。有效: observe|suggest|approve|autonomous", cfg.Daemon.AutonomyLevel)
	}
	if cfg.LLM.Provider == "" {
		return fmt.Errorf("llm.provider 不能为空")
	}
	if cfg.LLM.Model == "" {
		return fmt.Errorf("llm.model 不能为空")
	}
	if cfg.LLM.BaseURL != "" && !strings.HasPrefix(cfg.LLM.BaseURL, "http") {
		return fmt.Errorf("llm.base_url 必须以 http:// 或 https:// 开头，当前: %s", cfg.LLM.BaseURL)
	}
	return nil
}

// WriteDefault 写入带注释的默认配置文件（v1.1.0 首次启动自动生成）。

// LoadTest 加载测试配置（优先 daemon.test.yaml，回退 daemon.yaml）。
// 测试配置可含真实 API Key——.gitignore 已保护，不会提交到 Git。
func LoadTest(configDir string) (*Config, error) {
	testPath := configDir + "/daemon.test.yaml"
	if _, err := os.Stat(testPath); err == nil {
		return Load(testPath)
	}
	return Load(configDir + "/daemon.yaml")
}
func WriteDefault(path string) error {
	defaultYAML := `# GoalOS 配置文件 — 首次启动自动生成
# 修改后保存，然后执行: curl -X POST http://localhost:18920/api/system/reload

daemon:
  port: 18920               # HTTP 端口
  autonomy_level: approve    # observe|suggest|approve|autonomous

llm:
  provider: openai           # LLM 供应商: openai|anthropic|ollama
  model: glm-5.1             # 模型名称
  api_key: ""                # API Key。直接填写或留空使用环境变量
  base_url: ""               # API 地址。空=使用默认端点。示例: https://api.openai.com/v1
  max_tokens: 4096
  temperature: 0.3
  timeout: 300s

policy:
  approval_timeout: 300        # 审批超时秒数
  token_budget: 1000000        # 单 Goal Token 上限
  token_warning: 0.8           # 预算警告阈值
  auto_fix_max: 3              # 自修正最大次数
  flow_degrade_threshold: 3    # Flow 降级触发
  goal_anchor_interval: 20     # GoalAnchor 检查间隔
  recovery_retry_max: 3        # Recovery 重试最大

persona: concise             # concise|warm|minimal
`
	return os.WriteFile(path, []byte(defaultYAML), 0600)
}
