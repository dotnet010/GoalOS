//go:build prototype && windows

// appcontainer_aclmap_windows_test.go——default-deny 边界测绘（2026-08-31，顾问复审
// 决定性测试的意外副产品）：A 相「未授予不可执行」被实机推翻——Temp 内任意二进制
// 在 AppContainer 下未授予即可执行。模型与现实不符，测绘真实拒绝面：
// 读/执行/写 × Temp/home/Desktop/System32/ssh 全矩阵，逐格实证，不许理论。
package prototype

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"golang.org/x/sys/windows"
)

// TestAppContainer_ACLMap 拒绝面测绘矩阵（出数性质——每格一个实机事实）。
func TestAppContainer_ACLMap(t *testing.T) {
	const profile = "GoalOS-Spike-AC-Map"
	deleteAppContainer(profile)
	sid := createAppContainer(t, profile)
	defer deleteAppContainer(profile)
	defer windows.FreeSid(sid)

	home, _ := os.UserHomeDir()
	tmp := os.TempDir()
	cmd := `C:\Windows\System32\cmd.exe`

	// 靶标实体（真实文件——拒绝与「文件不存在」可分辨）
	targets := map[string]string{
		"temp-file":   filepath.Join(tmp, "goalos-acmap.txt"),
		"home-file":   filepath.Join(home, "goalos-acmap.txt"),
		"ssh-config":  filepath.Join(home, ".ssh", "config"),
		"sys32-hosts": `C:\Windows\System32\drivers\etc\hosts`,
	}
	for _, k := range []string{"temp-file", "home-file"} {
		if err := os.WriteFile(targets[k], []byte("probe"), 0644); err != nil {
			t.Fatal(err)
		}
		defer os.Remove(targets[k])
	}

	// 读矩阵（「拒」格必须带 OS 拒绝证据文本——exit code/成功缺席≠拒绝证据，
	// 复审隐藏问题(2)复查：sys32-cmd 单格补丁→全格同口径系统化）
	for _, k := range []string{"temp-file", "home-file", "ssh-config", "sys32-hosts"} {
		code, out := runInAppContainer(t, sid, cmd+` /c chcp 65001 >nul & type "`+targets[k]+`" 2>&1`, 30*time.Second)
		if strings.Contains(out, "probe") || strings.Contains(out, "127.0.0.1") || code == 0 && !evidenceDenied(out) {
			t.Logf("读 %-12s = 允许（内容命中/exit=0）", k)
			continue
		}
		if !evidenceDenied(out) {
			t.Fatalf("读 %s=非成功但无拒绝证据（疑似假阴性藏点）: exit=%d out=%q", k, code, out)
		}
		t.Logf("读 %-12s = 拒（证据=%q）", k, strings.TrimSpace(out))
	}

	// 执行矩阵：temp 内 exe（A 相已知=允许）vs system32 exe vs home exe
	exeTargets := map[string]string{
		"temp-exe": buildDynamicBinary(t, t.TempDir(), "mapexec"), // t.TempDir=temp 内
	}
	homeExe := buildDynamicBinary(t, filepath.Join(home, "goalos-acmap-bin"), "homebin")
	defer os.RemoveAll(filepath.Join(home, "goalos-acmap-bin"))
	exeTargets["home-exe"] = homeExe
	exeTargets["sys32-cmd"] = `C:\Windows\System32\compattelrunner.exe` // 普通系统 exe（非控制台宿主）
	// 执行矩阵：temp/home exe（启动面）。语义诚实注记：CreateProcess 失败=测试
	// 当场死（runInAppContainer Fatalf）——能走到本行=启动已成功，故本矩阵
	// 只可记录「允许」；进程自身退出码（如 0x80070057 参数校验）≠启动被拒
	// （sys32-cmd 格教训——exit code≠强制证据，复审(3)复查锚点）。
	for _, k := range []string{"temp-exe", "home-exe", "sys32-cmd"} {
		code, out := runInAppContainer(t, sid, `"`+exeTargets[k]+`"`, 30*time.Second)
		t.Logf("执行 %-10s = 允许（启动面成立——CreateProcess 成功；exit=%d 为进程自身语义 out=%q）",
			k, code, strings.TrimSpace(out))
	}

	// 写矩阵（同口径：「拒」格必须带 OS 拒绝证据文本）
	for _, k := range []string{"temp-file", "home-file"} {
		code, out := runInAppContainer(t, sid, cmd+` /c chcp 65001 >nul & echo x >> "`+targets[k]+`" 2>&1`, 30*time.Second)
		if code == 0 {
			t.Logf("写 %-12s = 允许", k)
			continue
		}
		if !evidenceDenied(out) {
			t.Fatalf("写 %s=非零退出但无拒绝证据（疑似假阴性藏点）: exit=%d out=%q", k, code, out)
		}
		t.Logf("写 %-12s = 拒（证据=%q）", k, strings.TrimSpace(out))
	}
}
