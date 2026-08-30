//go:build prototype && windows

// appcontainer_dynamic_windows_test.go——顾问复审第 1 条决定性 RED（2026-08-31）：
// 「AppContainer 能力模型能否兜住运行时动态选择二进制/资源」——R-1648 定案前
// 唯一决定性测试（优先级高于 WritableRoots 细节——顾问复审采纳项）。
//
// 实机模型修正（ACLMap 测绘先行结论）：CreateProcess 语义=父进程 token 打开
// image——执行面默认放行（无需授予）；读/写=运行期子进程 AC token 语义——默认拒绝。
// 故断言形态（GoalOS 真实场景映射：shell.execute=运行时任意二进制+生成代码）：
//  A. 未授予：动态二进制可执行（启动面）但读自身目录数据=拒（运行期面）——
//     「执行≠读」分离，正是受限档所需形态；
//  B. 授予（OI)(CI)(M)：工作区内读数据+写文件合法（WritableRoots 语义）；
//  C. 授予后写工作区外（用户 home）仍被拒（授予不扩面）；
//  D. 工具目录资源授予：未授予读数据=拒；授予 RX 后读通——
//     「运行时动态资源授予」机制成立（python 族 DLL/数据读取场景映射）。
//
// 授予载体=icacls（spike 期——生产实现收编为 Go ACL API，同 agentbox acl.go 族）。
package prototype

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
	"unsafe"

	"golang.org/x/sys/windows"
)

// helloSource 动态二进制源（-writeout=写探针；-readin=读探针——运行期数据面）。
const helloSource = `package main

import (
	"fmt"
	"os"
)

func main() {
	fmt.Println("hello-ac-ok")
	if len(os.Args) > 1 && os.Args[1] == "-writeout" {
		err := os.WriteFile(os.Args[2], []byte("ac-write"), 0644)
		if err != nil {
			fmt.Println("WRITE-DENIED: " + err.Error())
			os.Exit(2)
		}
		fmt.Println("WRITE-OK")
	}
	if len(os.Args) > 1 && os.Args[1] == "-readin" {
		data, err := os.ReadFile(os.Args[2])
		if err != nil {
			fmt.Println("READ-DENIED: " + err.Error())
			os.Exit(3)
		}
		fmt.Println("READ-OK: " + string(data))
	}
}
`

// buildDynamicBinary 测试期实时构建「运行时动态选择」二进制（go 工具链在场=本测试前提）。
func buildDynamicBinary(t *testing.T, dir, name string) string {
	t.Helper()
	if err := os.MkdirAll(dir, 0755); err != nil {
		t.Fatal(err)
	}
	src := filepath.Join(dir, name+".go")
	if err := os.WriteFile(src, []byte(helloSource), 0644); err != nil {
		t.Fatal(err)
	}
	exe := filepath.Join(dir, name+".exe")
	out, err := exec.Command("go", "build", "-o", exe, src).CombinedOutput()
	if err != nil {
		t.Fatalf("动态二进制构建失败（go 工具链前提）: %v\n%s", err, out)
	}
	return exe
}

// sidString AppContainer SID→字符串（icacls *SID 语法载体）。
func sidString(t *testing.T, sid *windows.SID) string {
	t.Helper()
	var s *uint16
	if err := windows.ConvertSidToStringSid(sid, &s); err != nil {
		t.Fatalf("ConvertSidToStringSid: %v", err)
	}
	defer windows.LocalFree(windows.Handle(unsafe.Pointer(s)))
	return windows.UTF16PtrToString(s)
}

// icaclsGrant 授予/回收 ACE（prod 注记：收编 Go ACL API 后此载体退役——spike 期诚实标注）。
func icaclsGrant(t *testing.T, sidStr, path, perm string, remove bool) {
	t.Helper()
	args := []string{path}
	if remove {
		args = append(args, "/remove:g", "*"+sidStr)
	} else {
		args = append(args, "/grant", "*"+sidStr+":"+perm)
	}
	out, err := exec.Command("icacls", args...).CombinedOutput()
	if err != nil {
		t.Fatalf("icacls %v: %v\n%s", args, err, out)
	}
	t.Logf("icacls %s %s → %s", path, map[bool]string{true: "remove", false: "grant " + perm}[remove], strings.TrimSpace(string(out)))
}

// TestAppContainer_DynamicBinary 顾问复审决定性测试——动态二进制/资源授予面四相
// （实机模型修正后：执行=启动面父 token；读写=运行期 AC token）。
func TestAppContainer_DynamicBinary(t *testing.T) {
	const profile = "GoalOS-Spike-AC-Dyn"
	deleteAppContainer(profile)
	sid := createAppContainer(t, profile)
	defer deleteAppContainer(profile)
	defer windows.FreeSid(sid)
	sidStr := sidString(t, sid)
	t.Logf("AppContainer SID=%s", sidStr)

	base := t.TempDir()
	ws := filepath.Join(base, "workspace")
	toolDir := filepath.Join(base, "usertool") // 模拟 per-user 工具目录
	for _, d := range []string{ws, toolDir} {
		if err := os.MkdirAll(d, 0755); err != nil {
			t.Fatal(err)
		}
	}
	wsExe := buildDynamicBinary(t, ws, "hello")
	toolExe := buildDynamicBinary(t, toolDir, "tool")
	// 运行期数据靶标（工具目录的 DLL/配置 场景映射）
	wsData := filepath.Join(ws, "data.txt")
	toolData := filepath.Join(toolDir, "data.txt")
	for _, f := range []string{wsData, toolData} {
		if err := os.WriteFile(f, []byte("payload"), 0644); err != nil {
			t.Fatal(err)
		}
	}

	// —— A. 未授予：执行=允许（启动面）；读自身目录数据=拒（运行期面）——
	code, out := runInAppContainer(t, sid, `"`+wsExe+`"`, 30*time.Second)
	if code != 0 || !strings.Contains(out, "hello-ac-ok") {
		t.Fatalf("A：未授予动态二进制执行失败（启动面应放行——CreateProcess 父 token 语义）: exit=%d %q", code, out)
	}
	t.Logf("A 未授予执行=允许（%q）", strings.TrimSpace(out))
	code, out = runInAppContainer(t, sid, `"`+wsExe+`" -readin "`+wsData+`"`, 30*time.Second)
	if code == 0 || !strings.Contains(out, "READ-DENIED") {
		t.Fatalf("A CRITICAL：未授予目录数据竟可读——读禁闭失效: exit=%d %q", code, out)
	}
	t.Logf("A 未授予读数据=拒绝（执行≠读——运行期面成立）")

	// —— B. 授予（OI)(CI)(M)：工作区内读+写合法 ——
	icaclsGrant(t, sidStr, ws, "(OI)(CI)(M)", false)
	defer icaclsGrant(t, sidStr, ws, "", true)
	code, out = runInAppContainer(t, sid, `"`+wsExe+`" -readin "`+wsData+`"`, 30*time.Second)
	if code != 0 || !strings.Contains(out, "READ-OK") {
		t.Fatalf("B：授予后工作区读失败（WritableRoots 语义未达成）: exit=%d %q", code, out)
	}
	wsWriteTarget := filepath.Join(ws, "written-by-ac.txt")
	code, out = runInAppContainer(t, sid, `"`+wsExe+`" -writeout "`+wsWriteTarget+`"`, 30*time.Second)
	if code != 0 || !strings.Contains(out, "WRITE-OK") {
		t.Fatalf("B：授予后工作区写失败: exit=%d %q", code, out)
	}
	if _, err := os.Stat(wsWriteTarget); err != nil {
		t.Fatalf("B：工作区写产物不可见: %v", err)
	}
	t.Logf("B 授予后工作区读+写=合法")

	// —— C. 写工作区外（用户 home）仍拒 ——
	home, _ := os.UserHomeDir()
	outTarget := filepath.Join(home, "goalos-ac-dyn-probe.txt")
	_ = os.Remove(outTarget)
	code, out = runInAppContainer(t, sid, `"`+wsExe+`" -writeout "`+outTarget+`"`, 30*time.Second)
	if code == 0 {
		_ = os.Remove(outTarget)
		t.Fatalf("C CRITICAL：写工作区外成功——授予扩面")
	}
	if !strings.Contains(out, "WRITE-DENIED") {
		t.Fatalf("C：写区外非拒绝形态: exit=%d %q", code, out)
	}
	t.Logf("C 写工作区外=拒绝（授予不扩面）")

	// —— D. 工具目录资源授予：未授予读数据=拒；授予 RX=读通 ——
	code, out = runInAppContainer(t, sid, `"`+toolExe+`" -readin "`+toolData+`"`, 30*time.Second)
	if code == 0 {
		t.Fatalf("D CRITICAL：未授予工具目录数据竟可读")
	}
	t.Logf("D 未授予工具数据读=拒绝")
	icaclsGrant(t, sidStr, toolDir, "(OI)(CI)(RX)", false)
	defer icaclsGrant(t, sidStr, toolDir, "", true)
	code, out = runInAppContainer(t, sid, `"`+toolExe+`" -readin "`+toolData+`"`, 30*time.Second)
	if code != 0 || !strings.Contains(out, "READ-OK") {
		t.Fatalf("D：RX 授予后工具数据仍不可读——动态资源授予机制不成立: exit=%d %q", code, out)
	}
	t.Logf("D 授予 RX 后工具数据读=通（%q）", strings.TrimSpace(out))
}
