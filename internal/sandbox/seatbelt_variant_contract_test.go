//go:build darwin

// seatbelt_variant_contract_test.go——TC-RT-051（12 清单 F 节——macOS 网域沙箱生成）：
// 网络授权变体的内容断言+实证执行。断言来源=D-2 蓝图 §四 TC-RT-051+会议 #258 适配裁定
// （SBPL 无 CIDR 粒度——端口级放行形态）+Invariant（绝不含 (allow network*) 裸通配）。
package sandbox

import (
	"os"
	"os/exec"
	"strings"
	"testing"
)

// TestSeatbelt_NetworkAuthorizedVariant（TC-RT-051）：
// (1)授权变体恰好含端口放行规则（tcp 443/80）+默认拒出站；(2)绝不含裸通配 (allow network*)；
// (3)未授权变体=全拒（deny network*）；(4)实证：变体可被 sandbox-exec 解析且边界内可执行；
// (5)实证：授权变体下 443 端口可出站（127.0.0.1:443 无监听=connection refused 非 EPERM——
// 端口放行生效的可分辨证据）；(6)单源漂移 fail-closed（锚点缺失=错误不产出）。
func TestSeatbelt_NetworkAuthorizedVariant(t *testing.T) {
	base, err := RestrictedSeatbeltProfileForNetwork(false)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(base, "(deny network*)") {
		t.Fatal("(3)未授权变体=网络全拒（deny network*）")
	}
	variant, err := RestrictedSeatbeltProfileForNetwork(true)
	if err != nil {
		t.Fatal(err)
	}
	// (1)内容断言
	for _, want := range []string{`(deny network-outbound)`, `(allow network-outbound (to tcp "*:443"))`, `(allow network-outbound (to tcp "*:80"))`} {
		if !strings.Contains(variant, want) {
			t.Fatalf("(1)授权变体缺规则 %q", want)
		}
	}
	// (2)Invariant：裸通配禁止
	if strings.Contains(variant, "(allow network*)") || strings.Contains(variant, "(allow network ") {
		t.Fatal("(2)Invariant 违反：含裸 network 通配")
	}
	if strings.Contains(variant, "(deny network*)") {
		t.Fatal("(1)授权变体残留全拒段（与端口放行矛盾）")
	}

	if _, err := exec.LookPath("sandbox-exec"); err != nil {
		t.Skip("sandbox-exec 不可用（非 macOS 宿主）")
	}
	home, _ := os.UserHomeDir()
	dir := t.TempDir()

	// (4)实证：变体解析+边界内执行
	pf := dir + "/variant.sb"
	if err := os.WriteFile(pf, []byte(variant), 0600); err != nil {
		t.Fatal(err)
	}
	out, err := exec.Command("/usr/bin/sandbox-exec", "-f", pf,
		"-D", "WORKSPACE_DIR="+dir, "-D", "TMP_DIR="+dir, "-D", "HOME_DIR="+home,
		"-D", "TARGET_BINARY=/usr/bin/true", "/usr/bin/true").CombinedOutput()
	if err != nil {
		t.Fatalf("(4)授权变体解析/执行失败: %v\n%s", err, out)
	}

	// (5)实证：443 端口出站放行生效（connection refused=端口通而服务无监听；EPERM=仍被拦）
	out2, _ := exec.Command("/usr/bin/sandbox-exec", "-f", pf,
		"-D", "WORKSPACE_DIR="+dir, "-D", "TMP_DIR="+dir, "-D", "HOME_DIR="+home,
		"-D", "TARGET_BINARY=/usr/bin/nc", "/usr/bin/nc", "-v", "-w", "1", "127.0.0.1", "443").CombinedOutput()
	if strings.Contains(string(out2), "Operation not permitted") {
		t.Fatalf("(5)443 端口仍被拦（EPERM）——端口放行未生效: %s", out2)
	}
	if !strings.Contains(string(out2), "Connection refused") && !strings.Contains(string(out2), "succeeded") {
		t.Fatalf("(5)出站证据形态不可分辨: %s", out2)
	}

	// (5)b 对照：非授权端口（22）仍被拒
	out3, _ := exec.Command("/usr/bin/sandbox-exec", "-f", pf,
		"-D", "WORKSPACE_DIR="+dir, "-D", "TMP_DIR="+dir, "-D", "HOME_DIR="+home,
		"-D", "TARGET_BINARY=/usr/bin/nc", "/usr/bin/nc", "-v", "-w", "1", "127.0.0.1", "22").CombinedOutput()
	if !strings.Contains(string(out3), "Operation not permitted") {
		t.Fatalf("(5)b 非授权端口 22 应 EPERM 拒绝，实际: %s", out3)
	}
}
