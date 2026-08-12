// Config 校验链路契约测试（R-1060, 会议 #191 修正）。
// D-3/D-7/D-8 系统性修复：Validate() 曾是死代码——Load/Reload 均不调用；
// approval_timeout 负值穿透（AfterFunc(负值)→立即超时→审批全线秒拒）。
// 契约：三条加载路径（Load/LoadTest 经 Load/Reload）全部过 Validate，
// 非法配置在任何路径上都必须拒绝（AWS STS MaxSessionDuration 语义：请求时校验）。
package config_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/goalos/goalos/internal/config"
)

// writeTempConfig 写临时 daemon.yaml，返回路径。
func writeTempConfig(t *testing.T, content string) string {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "daemon.yaml")
	if err := os.WriteFile(path, []byte(content), 0600); err != nil {
		t.Fatalf("write temp config: %v", err)
	}
	return path
}

const validBaseConfig = `llm:
  provider: anthropic
  model: claude-test
  base_url: https://api.example.com/v1
daemon:
  port: 18920
  autonomy_level: approve
`

// TestConfig_LoadValidates 启动路径必须过 Validate——非法 port 被 Load 拒绝。
// 注意：port: 0 会被 merge 语义（!= 0 = 未设置）吞为默认值，非显式非法；
// 用非零越界值 70000 才是用户显式非法输入。
func TestConfig_LoadValidates(t *testing.T) {
	path := writeTempConfig(t, `daemon:
  port: 70000
llm:
  provider: anthropic
  model: claude-test
  base_url: https://api.example.com/v1
`)
	_, err := config.Load(path)
	if err == nil {
		t.Fatal("Load 必须拒绝非法配置（port: 70000）——Validate() 曾是死代码（D-8）")
	}
}

// TestConfig_ReloadValidates 热重载路径必须过 Validate——非法值被 Reload 拒绝，
// 不得静默换入运行中 daemon（D-7）。
func TestConfig_ReloadValidates(t *testing.T) {
	path := writeTempConfig(t, validBaseConfig)
	cfg, err := config.Load(path)
	if err != nil {
		t.Fatalf("valid config must load: %v", err)
	}

	bad := writeTempConfig(t, validBaseConfig+`
policy:
  approval_timeout: -5
`)
	if err := cfg.Reload(bad); err == nil {
		t.Fatal("Reload 必须拒绝非法配置（approval_timeout: -5）")
	}
}

// TestConfig_ValidateRejectsNonPositiveApprovalTimeout
// approval_timeout ≤ 0 必须拒绝——负值穿透 = AfterFunc 立即触发 = 审批不可用。
func TestConfig_ValidateRejectsNonPositiveApprovalTimeout(t *testing.T) {
	path := writeTempConfig(t, validBaseConfig+`
policy:
  approval_timeout: -5
`)
	if _, err := config.Load(path); err == nil {
		t.Fatal("approval_timeout: -5 必须被拒绝（D-3）")
	}
}

// TestConfig_ValidateRejectsNonPositiveTokenTTL
// token_ttl ≤ 0 必须拒绝——token TTL 为行动执行窗口，非法值 fail-closed。
// 注意：token_ttl: 0 会被 merge 语义吞为默认值（见 TestConfig_LoadValidates 注释），
// 用 -5 走显式非法路径。
func TestConfig_ValidateRejectsNonPositiveTokenTTL(t *testing.T) {
	path := writeTempConfig(t, validBaseConfig+`
policy:
  token_ttl: -5
`)
	if _, err := config.Load(path); err == nil {
		t.Fatal("token_ttl: -5 必须被拒绝")
	}
}

// TestConfig_TokenTTLDefault token_ttl 默认 300s（与审批窗口同尺度，R-1059）。
func TestConfig_TokenTTLDefault(t *testing.T) {
	cfg := config.Default()
	if cfg.Policy.TokenTTL != 300 {
		t.Errorf("expected default token_ttl 300, got %d", cfg.Policy.TokenTTL)
	}
}
