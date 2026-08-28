// env_whitelist_contract_test.go——TC-RT-031c（12 清单 F 节 TC-RT-031 三子句之 c——
// 密钥材料不跨进程传递=子进程 env 构造侧隔离断言；R-1557 口径：/proc 事后读取 darwin
// 不可用弃用，构造侧断言=唯一载体）。断言来源=06 §X.8 密钥纪律+Kees N16 既有修复。
package pluginrunner

import (
	"strings"
	"testing"
)

// TestChildEnv_Whitelist 子进程环境白名单（构造侧）：
// ①env=固定四键白名单（PATH/HOME/GOALOS_WORKSPACE/GOALOS_TMP）
// ②密钥材料变量（GOALOS_SECRET_KEY/daemon_secret/secrets.key 内容/Token 明文）永不入列
// ③daemon 侧注入敏感变量后白名单不变（构造与环境无关——白名单硬编码非透传）。
func TestChildEnv_Whitelist(t *testing.T) {
	// ③daemon 侧注入敏感变量（模拟泄漏场景——构造侧必须不受影响）
	t.Setenv("GOALOS_SECRET_KEY", "supersecret")
	t.Setenv("DAEMON_SECRET", "alsosensitive")

	env := childEnv(ExecConfig{WorkDir: "/tmp/ws", TmpDir: "/tmp/tmp"})

	// ①白名单键集钉死（四键，顺序无关）
	allowed := map[string]bool{"PATH": true, "HOME": true, "GOALOS_WORKSPACE": true, "GOALOS_TMP": true}
	if len(env) != 4 {
		t.Fatalf("白名单键数=%d 应=4（%v）", len(env), env)
	}
	for _, kv := range env {
		key, _, _ := strings.Cut(kv, "=")
		if !allowed[key] {
			t.Fatalf("白名单外变量泄漏入子进程环境: %q", key)
		}
	}
	// ②敏感材料键与值均不出现
	for _, kv := range env {
		if strings.Contains(kv, "supersecret") || strings.Contains(kv, "alsosensitive") ||
			strings.Contains(strings.ToUpper(kv), "SECRET") || strings.Contains(strings.ToUpper(kv), "TOKEN") {
			t.Fatalf("敏感材料入子进程环境: %q", kv)
		}
	}
	// 防锚定退化：白名单值实际生效（PATH 非空）
	seen := map[string]bool{}
	for _, kv := range env {
		k, v, _ := strings.Cut(kv, "=")
		seen[k] = v != ""
	}
	if !seen["PATH"] || !seen["GOALOS_WORKSPACE"] {
		t.Fatalf("白名单值未生效: %v", env)
	}
}
