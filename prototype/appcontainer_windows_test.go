//go:build prototype && windows

// appcontainer_windows_test.go——AppContainer 第一性 spike（2026-08-30，PM 指令：
// 直面问题非逃避——agentbox Tier1 无网络/读禁闭机制实证后，验证 Windows 非管理员态
// 真边界原语可行性）。
//
// 第一性断言（全部实机验证，非文档声称）：
//  ①网络禁闭：零 named capability 的 AppContainer 进程出站连接=WSAEACCES 即时拒绝
//    （内核 WFP 强制——与"路由不可达超时"可分辨，反虚假绿）；
//  ②敏感读禁闭：AppContainer 进程读 ~/.ssh\config=Access is denied
//    （用户 profile 未授予 ALL APPLICATION PACKAGES——OS 默认拒绝，非配置声明）；
//  ③fs 写禁闭：AppContainer 进程写 C:\Windows=Access is denied；
//  ④边界内执行：cmd /c echo 真实执行+输出回传（非 execvp 级假象）；
//  ⑤边界内合法读：读 System32 内文件成功（ALL APPLICATION PACKAGES 默认授予面）。
//
// 非管理员可行性（第一性关键）：CreateAppContainerProfile/进程属性化不需管理员——
// 与 agentbox Tier2（沙箱用户+防火墙=管理员）路线本质区别。
package prototype

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
	"time"
	"unsafe"

	"golang.org/x/sys/windows"
)

const (
	procThreadAttributeSecurityCapabilities = 0x00020009 // ProcThreadAttributeValue(9,FALSE,TRUE,FALSE)
	extendedStartupinfoPresent              = 0x00080000
	startfUseStdHandles                     = 0x00000100
)

// securityCapabilities SECURITY_CAPABILITIES（winnt.h——amd64 布局 24B）。
type securityCapabilities struct {
	appContainerSid *windows.SID
	capabilities    uintptr // *sidAndAttributes——零 capability=无网络
	capabilityCount uint32
	reserved        uint32
}

var (
	userenv = windows.NewLazySystemDLL("userenv.dll")
	kern32  = windows.NewLazySystemDLL("kernel32.dll")

	procCreateAppContainerProfile = userenv.NewProc("CreateAppContainerProfile")
	procDeleteAppContainerProfile = userenv.NewProc("DeleteAppContainerProfile")
	procFlushFileBuffers          = kern32.NewProc("FlushFileBuffers")
)

// createAppContainer 创建 profile（幂等：已存在=HRESULT 0x800700B7 视为成功）+返回 SID。
func createAppContainer(t *testing.T, name string) *windows.SID {
	t.Helper()
	namePtr, _ := windows.UTF16PtrFromString(name)
	var sid *windows.SID
	r, _, _ := procCreateAppContainerProfile.Call(
		uintptr(unsafe.Pointer(namePtr)),
		uintptr(unsafe.Pointer(namePtr)), // displayName
		uintptr(unsafe.Pointer(namePtr)), // description
		0, 0, // 零 named capability=无网络能力（第一性断言①的载体）
		uintptr(unsafe.Pointer(&sid)),
	)
	// 0x800700B7=ERROR_ALREADY_EXISTS（幂等重入）；此时 SID 未出参=derive 获取
	if r == 0x800700B7 {
		r = 0
		var err error
		sid, err = deriveAppContainerSid(name)
		if err != nil {
			t.Fatalf("DeriveAppContainerSidFromAppContainerName: %v", err)
		}
	}
	if int32(r) != 0 {
		t.Fatalf("CreateAppContainerProfile HRESULT=0x%08x", uint32(r))
	}
	return sid
}

func deriveAppContainerSid(name string) (*windows.SID, error) {
	proc := kern32.NewProc("DeriveAppContainerSidFromAppContainerName")
	namePtr, _ := windows.UTF16PtrFromString(name)
	var sid *windows.SID
	r, _, _ := proc.Call(uintptr(unsafe.Pointer(namePtr)), uintptr(unsafe.Pointer(&sid)))
	if int32(r) != 0 {
		return nil, syscall.Errno(r)
	}
	return sid, nil
}

func deleteAppContainer(name string) {
	namePtr, _ := windows.UTF16PtrFromString(name)
	procDeleteAppContainerProfile.Call(uintptr(unsafe.Pointer(namePtr)))
}

// runInAppContainer 以 AppContainer token 启动命令，stdout+stderr 合并捕获，返回退出码与输出。
func runInAppContainer(t *testing.T, sid *windows.SID, cmdline string, timeout time.Duration) (int, string) {
	t.Helper()

	// stdout/stderr 捕获=临时文件重定向（继承句柄——避免匿名管道在 AppContainer 下的边角）
	outFile := filepath.Join(t.TempDir(), "out.txt")
	outPathPtr, _ := windows.UTF16PtrFromString(outFile)
	sa := &windows.SecurityAttributes{Length: uint32(unsafe.Sizeof(windows.SecurityAttributes{})), InheritHandle: 1}
	hOut, err := windows.CreateFile(outPathPtr, windows.GENERIC_WRITE, windows.FILE_SHARE_READ, sa, windows.CREATE_ALWAYS, windows.FILE_ATTRIBUTE_NORMAL, 0)
	if err != nil {
		t.Fatalf("CreateFile 捕获文件: %v", err)
	}
	defer windows.CloseHandle(hOut)

	secCaps := securityCapabilities{appContainerSid: sid}
	attrList, err := windows.NewProcThreadAttributeList(1)
	if err != nil {
		t.Fatalf("NewProcThreadAttributeList: %v", err)
	}
	defer attrList.Delete()
	if err := attrList.Update(
		procThreadAttributeSecurityCapabilities,
		unsafe.Pointer(&secCaps), unsafe.Sizeof(secCaps),
	); err != nil {
		t.Fatalf("UpdateProcThreadAttribute SECURITY_CAPABILITIES: %v", err)
	}

	var siex windows.StartupInfoEx
	siex.Cb = uint32(unsafe.Sizeof(siex))
	siex.Flags = startfUseStdHandles
	siex.StdOutput = hOut
	siex.StdErr = hOut
	siex.ProcThreadAttributeList = attrList.List()

	cmdPtr, _ := windows.UTF16PtrFromString(cmdline)
	var pi windows.ProcessInformation
	err = windows.CreateProcess(
		nil, cmdPtr, nil, nil, true,
		windows.CREATE_SUSPENDED|extendedStartupinfoPresent|windows.CREATE_UNICODE_ENVIRONMENT,
		nil, nil, &siex.StartupInfo, &pi,
	)
	if err != nil {
		t.Fatalf("CreateProcess(AppContainer): %v", err)
	}
	defer windows.CloseHandle(pi.Process)
	defer windows.CloseHandle(pi.Thread)
	if _, err := windows.ResumeThread(pi.Thread); err != nil {
		t.Fatalf("ResumeThread: %v", err)
	}
	wait, err := windows.WaitForSingleObject(pi.Process, uint32(timeout.Milliseconds()))
	if err != nil || wait == uint32(windows.WAIT_TIMEOUT) {
		_ = windows.TerminateProcess(pi.Process, 1)
		t.Fatalf("进程等待失败/超时: wait=%d err=%v", wait, err)
	}
	var code uint32
	if err := windows.GetExitCodeProcess(pi.Process, &code); err != nil {
		t.Fatalf("GetExitCodeProcess: %v", err)
	}
	// 冲刷句柄后读回（CreateFile 写句柄仍在——CloseHandle 由 defer 收尾前先 FlushFileBuffers）
	kern32.NewProc("FlushFileBuffers").Call(uintptr(hOut))
	data, _ := os.ReadFile(outFile)
	return int(code), string(data)
}

// evidenceDenied OS 拒绝证据（本地化鲁棒——英文 MUI/中文 MUI UTF-8/GBK 三形态）。
func evidenceDenied(out string) bool {
	if strings.Contains(out, "Access is denied") || strings.Contains(out, "拒绝") {
		return true
	}
	// GBK「拒绝」原始字节（cmd 中文 MUI 未 chcp 时）
	return strings.Contains(out, "\xbe\xdc\xbe\xf8")
}

// evidenceNetDenied WSAEACCES 证据（英/中 UTF-8/GBK 三形态——与 10060/10065 假象分辨）。
func evidenceNetDenied(out string) bool {
	if strings.Contains(out, "forbidden by its access permissions") || strings.Contains(out, "访问权限") {
		return true
	}
	// GBK「访问权限」原始字节
	return strings.Contains(out, "\xb7\xc3\xce\xca\xc8\xa8\xcf\xde")
}

// TestAppContainer_FirstPrinciples 第一性五断言实机验证（spike 出数性质——R-1478③）。
func TestAppContainer_FirstPrinciples(t *testing.T) {
	const profile = "GoalOS-Spike-AC"
	deleteAppContainer(profile) // 幂等清理前次残留
	sid := createAppContainer(t, profile)
	defer deleteAppContainer(profile)

	home, _ := os.UserHomeDir()
	cmd := `C:\Windows\System32\cmd.exe`

	// ④边界内执行（先证非 execvp 假象——进程真实启动+输出回传）
	code, out := runInAppContainer(t, sid, cmd+` /c echo spike-ok`, 30*time.Second)
	if code != 0 || !strings.Contains(out, "spike-ok") {
		t.Fatalf("④边界内执行失败（exit=%d）——AppContainer 启动面未成立: %q", code, out)
	}
	t.Logf("④边界内执行=成功输出 %q", strings.TrimSpace(out))

	// ③fs 写禁闭：写 C:\Windows 必拒
	// chcp 65001：本机中文 MUI 下 cmd 错误文本=GBK「被拒绝访问」——统一 UTF-8 再断言
	code, out = runInAppContainer(t, sid, cmd+` /c chcp 65001 >nul & echo x > C:\Windows\goalos-ac-probe.txt`, 30*time.Second)
	if code == 0 {
		t.Fatalf("③CRITICAL：写 C:\\Windows 成功——AppContainer 写禁闭失效: %q", out)
	}
	if !evidenceDenied(out) {
		t.Fatalf("③写拒绝非 OS 证据形态（应含 Access is denied/拒绝）: %q", out)
	}
	t.Logf("③写 C:\\Windows=拒绝（%q）", strings.TrimSpace(out))

	// ②敏感读禁闭：读 ~/.ssh\config 必拒（profile 未授 ALL APPLICATION PACKAGES）
	sshCfg := filepath.Join(home, ".ssh", "config")
	if _, err := os.Stat(sshCfg); err != nil {
		t.Skipf("②无 ~/.ssh/config 实体——读禁闭探针无靶标（本机环境）")
	}
	code, out = runInAppContainer(t, sid, cmd+` /c chcp 65001 >nul & type "`+sshCfg+`"`, 30*time.Second)
	if code == 0 {
		t.Fatalf("②CRITICAL：读 ~/.ssh/config 成功——AppContainer 读禁闭失效")
	}
	if !evidenceDenied(out) {
		t.Fatalf("②读拒绝非 OS 证据形态: %q", out)
	}
	t.Logf("②读 ~/.ssh/config=拒绝（%q）", strings.TrimSpace(out))

	// ①网络禁闭：零 capability=出站 WSAEACCES 即时拒绝（与超时/无路由可分辨）
	psCmd := `C:\Windows\System32\WindowsPowerShell\v1.0\powershell.exe`
	netProbe := ` -NoProfile -Command "try { $c=New-Object Net.Sockets.TcpClient; $c.Connect('192.0.2.1',80); exit 1 } catch { Write-Output $_.Exception.InnerException.Message; exit 3 }"`
	code, out = runInAppContainer(t, sid, psCmd+netProbe, 60*time.Second)
	if code == 1 {
		t.Fatalf("①CRITICAL：出站连接成功——AppContainer 网络禁闭失效")
	}
	if code != 3 {
		t.Fatalf("①探针形态异常（exit=%d 应=3 捕获分支）: %q", code, out)
	}
	// WSAEACCES=10013「以其访问权限不允许的方式访问套接字」=OS 强制证据；
	// 超时/无路由（10060/10065）=非边界证据——反虚假绿关键分辨面（本地化鲁棒匹配）
	if !evidenceNetDenied(out) {
		t.Fatalf("①网络拒绝非 WSAEACCES 证据（疑似超时/无路由假象）: %q", out)
	}
	t.Logf("①出站 192.0.2.1:80=WSAEACCES 拒绝（%q）", strings.TrimSpace(out))

	// ⑤边界内合法读：System32 文件可读（默认授予面——边界不是"全锁死"）
	code, out = runInAppContainer(t, sid, cmd+` /c type C:\Windows\System32\drivers\etc\hosts`, 30*time.Second)
	if code != 0 {
		t.Fatalf("⑤边界内合法读失败（应可读系统目录）: %q", out)
	}
	t.Logf("⑤边界内读 System32=成功（%d 字节）", len(out))

	// 对照组：同一探针在 AppContainer 外（主进程）——网络超时/无路由语义区分
	extOut, _ := exec.Command(psCmd, "-NoProfile", "-Command",
		"try { $c=New-Object Net.Sockets.TcpClient; $c.Connect('192.0.2.1',80); 'CONNECTED' } catch { $_.Exception.InnerException.Message }").CombinedOutput()
	t.Logf("对照组（容器外）出站结果: %q", strings.TrimSpace(string(extOut)))
}
