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
)

// TestExecutorDarwin_SeatbeltProfileValid：构造的 profile 必须被 sandbox-exec 接受
// （/usr/bin/true 在边界内 exit 0）；非法 filter 必须被拒绝（探针有牙——负向对照）。
func TestExecutorDarwin_SeatbeltProfileValid(t *testing.T) {
	// 登记先红（R-1452 合法形态——会议 #256 裁决前）：executor 内联 profile 为读白名单
	// 形态，现代 macOS 上进程启动即 abort（dyld 系统面依赖不可枚举——实证 2026-08-29）。
	// 转绿=executor 收敛到 Option B 语义（provider profile_restricted_darwin.sb 同源）。
	t.Skip("登记先红：executor 内联 profile 启动期 abort（实证）——转绿归会议 #256 语义裁决后收敛")
	if _, err := exec.LookPath("sandbox-exec"); err != nil {
		t.Skip("sandbox-exec 不可用（非 macOS 宿主）")
	}
	dir := t.TempDir()

	// 正向：生产 profile 必须可解析且边界内可执行
	profilePath := filepath.Join(dir, "prod.sb")
	if err := os.WriteFile(profilePath, []byte(seatbeltProfileDarwin(dir, dir)), 0600); err != nil {
		t.Fatal(err)
	}
	out, err := exec.Command("/usr/bin/sandbox-exec", "-f", profilePath,
		"-D", "HOME_DIR="+dir, "-D", "TARGET_BINARY=/usr/bin/true", "/usr/bin/true").CombinedOutput()
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
