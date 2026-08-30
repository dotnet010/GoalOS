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

	// 读矩阵
	for _, k := range []string{"temp-file", "home-file", "ssh-config", "sys32-hosts"} {
		code, out := runInAppContainer(t, sid, cmd+` /c chcp 65001 >nul & type "`+targets[k]+`" >nul 2>&1 && echo READ-OK || echo READ-FAIL`, 30*time.Second)
		verdict := "拒"
		if strings.Contains(out, "READ-OK") {
			verdict = "允许"
		}
		t.Logf("读 %-12s = %s（exit=%d）", k, verdict, code)
	}

	// 执行矩阵：temp 内 exe（A 相已知=允许）vs system32 exe vs home exe
	exeTargets := map[string]string{
		"temp-exe": buildDynamicBinary(t, t.TempDir(), "mapexec"), // t.TempDir=temp 内
	}
	homeExe := buildDynamicBinary(t, filepath.Join(home, "goalos-acmap-bin"), "homebin")
	defer os.RemoveAll(filepath.Join(home, "goalos-acmap-bin"))
	exeTargets["home-exe"] = homeExe
	exeTargets["sys32-cmd"] = `C:\Windows\System32\compattelrunner.exe` // 普通系统 exe（非控制台宿主）
	for _, k := range []string{"temp-exe", "home-exe", "sys32-cmd"} {
		code, out := runInAppContainer(t, sid, `"`+exeTargets[k]+`"`, 30*time.Second)
		verdict := "拒"
		if strings.Contains(out, "hello-ac-ok") || code == 0 {
			verdict = "允许"
		}
		t.Logf("执行 %-10s = %s（exit=%d out=%q）", k, verdict, code, strings.TrimSpace(out))
	}

	// 写矩阵（复测锚定）
	for _, k := range []string{"temp-file", "home-file"} {
		code, _ := runInAppContainer(t, sid, cmd+` /c echo x >> "`+targets[k]+`"`, 30*time.Second)
		verdict := "拒"
		if code == 0 {
			verdict = "允许"
		}
		t.Logf("写 %-12s = %s（exit=%d）", k, verdict, code)
	}
}
