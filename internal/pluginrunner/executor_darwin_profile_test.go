//go:build darwin

// executor_darwin_profile_test.go——Seatbelt profile SBPL 合法性契约测试（darwin）。
// 依据：2026-08-29 实证事故——内联 profile 含非法 filter (deny fork)，从未通过
// sandbox-exec 解析（插件沙箱路径实际未生效）；测试隔离纪律：本机调度延迟不影响
// 本测试（纯进程执行无定时窗口）。
package pluginrunner

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/goalos/goalos/internal/sandbox"
)

// TestExecutorDarwin_SeatbeltProfileValid：构造的 profile 必须被 sandbox-exec 接受
// （/usr/bin/true 在边界内 exit 0）；非法 filter 必须被拒绝（探针有牙——负向对照）。
// 转绿（R-1641-3 收敛落地——2026-08-29）：executor profile=internal/sandbox 单源
// （Option B 语义），生产插件沙箱路径自此真实生效。
func TestExecutorDarwin_SeatbeltProfileValid(t *testing.T) {
	if _, err := exec.LookPath("sandbox-exec"); err != nil {
		t.Skip("sandbox-exec 不可用（非 macOS 宿主）")
	}
	dir := t.TempDir()

	// 正向：生产 profile 必须可解析且边界内可执行
	profilePath := filepath.Join(dir, "prod.sb")
	if err := os.WriteFile(profilePath, []byte(sandbox.RestrictedSeatbeltProfile()), 0600); err != nil {
		t.Fatal(err)
	}
	out, err := exec.Command("/usr/bin/sandbox-exec", "-f", profilePath,
		"-D", "WORKSPACE_DIR="+dir, "-D", "TMP_DIR="+dir, "-D", "HOME_DIR="+dir,
		"-D", "TARGET_BINARY=/usr/bin/true", "/usr/bin/true").CombinedOutput()
	if err != nil {
		t.Fatalf("生产 profile 未通过 sandbox-exec 解析/执行（插件沙箱路径失效事故类）: %v\n%s", err, out)
	}

	// 负向：非法 filter 必须被拒（验证本测试有牙）
	badPath := filepath.Join(dir, "bad.sb")
	if err := os.WriteFile(badPath, []byte("(version 1)\n(deny default)\n(deny fork)\n"), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := exec.Command("/usr/bin/sandbox-exec", "-f", badPath, "/usr/bin/true").CombinedOutput(); err == nil {
		t.Fatal("负向对照失效：非法 filter (deny fork) 未被拒绝——本测试无牙")
	}
}
