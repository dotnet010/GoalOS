// builtin_profile_contract_test.go——内置基础 profile+digest 契约测试（D-1 A 方案——
// 会议 #257 PM 裁决：编译结果按（能力集, risk, 平台, PolicyRevision）缓存；
// 复核环节永远独立重算不读缓存）。12 清单 G 节登记（实现同步补强——非先红标注）。
package sandbox

import "testing"

// TestBuiltinProfile_Deterministic digest 确定性：同输入集恒同 digest（缓存正确性前提）；
// 输入顺序无关（caps 排序入摘要）。
func TestBuiltinProfile_Deterministic(t *testing.T) {
	d1, _, err := BuiltinProfileDigest("I2", []string{"fs.read", "fs.write"}, "darwin", "builtin-v1")
	if err != nil {
		t.Fatal(err)
	}
	d2, _, err := BuiltinProfileDigest("I2", []string{"fs.write", "fs.read"}, "darwin", "builtin-v1") // 乱序
	if err != nil {
		t.Fatal(err)
	}
	if d1 != d2 {
		t.Fatalf("同输入集 digest 必须相同（顺序无关）：%q vs %q", d1, d2)
	}
	if len(d1) != 64 {
		t.Fatalf("digest 必须 hex(32B)=64 字符，实际 %d", len(d1))
	}
}

// TestBuiltinProfile_DigestSensitivity digest 敏感性：平台/策略版本/档位/能力集任一变化→digest 变化。
func TestBuiltinProfile_DigestSensitivity(t *testing.T) {
	base, _, _ := BuiltinProfileDigest("I2", []string{"fs.read"}, "darwin", "builtin-v1")
	byPlatform, _, _ := BuiltinProfileDigest("I2", []string{"fs.read"}, "linux", "builtin-v1")
	byRev, _, _ := BuiltinProfileDigest("I2", []string{"fs.read"}, "darwin", "builtin-v2")
	byTier, _, _ := BuiltinProfileDigest("I3", []string{"fs.read"}, "darwin", "builtin-v1")
	byCaps, _, _ := BuiltinProfileDigest("I2", []string{"web.search"}, "darwin", "builtin-v1")
	for name, d := range map[string]string{"平台": byPlatform, "策略版本": byRev, "档位": byTier, "能力集": byCaps} {
		if d == base {
			t.Fatalf("%s 变化必须改变 digest", name)
		}
	}
}

// TestBuiltinProfile_Semantics 语义派生：网络能力→allowlist 模式+能力族符号标记；
// 无网络能力→deny；任意子进程能力→MaxProcesses=64，否则=1；敏感目录禁读恒在；
// 写面=仅工作区/临时目录标记（Option B——R-1641）。
func TestBuiltinProfile_Semantics(t *testing.T) {
	_, raw, err := BuiltinProfileDigest("I2", []string{"web.search"}, "darwin", "builtin-v1")
	if err != nil {
		t.Fatal(err)
	}
	if raw.Network.Mode != "allowlist" || len(raw.Network.Allowlist) != 1 || raw.Network.Allowlist[0] != "web.search" {
		t.Fatalf("网络能力→allowlist 模式+能力族标记，实际: %+v", raw.Network)
	}
	_, raw2, _ := BuiltinProfileDigest("I2", []string{"fs.read"}, "linux", "builtin-v1")
	if raw2.Network.Mode != "deny" {
		t.Fatalf("无网络能力→deny，实际: %s", raw2.Network.Mode)
	}
	if raw2.Resources.MaxProcesses != 1 {
		t.Fatalf("无子进程能力→MaxProcesses=1，实际: %d", raw2.Resources.MaxProcesses)
	}
	_, raw3, _ := BuiltinProfileDigest("I2", []string{"shell.execute"}, "linux", "builtin-v1")
	if raw3.Resources.MaxProcesses != 64 {
		t.Fatalf("任意子进程能力→MaxProcesses=64，实际: %d", raw3.Resources.MaxProcesses)
	}
	// 敏感目录禁读恒在+写面=标记形态
	foundDeny := false
	for _, d := range raw2.Filesystem.Deny {
		if d == "$HOME/.ssh" {
			foundDeny = true
		}
	}
	if !foundDeny {
		t.Fatal("敏感目录禁读恒在（$HOME/.ssh 等四面）")
	}
	for _, w := range raw2.Filesystem.AllowWrite {
		if w != "$WORKSPACE" && w != "$GOALOS_TMP" {
			t.Fatalf("写面仅工作区/临时目录标记，实际含: %s", w)
		}
	}
}
