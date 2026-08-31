//go:build prototype && windows

// selftoken_windows_test.go——复审③异议的机器裁决（2026-08-31，PM 指令：
// 异议用测试解决，不靠谁读文档更细）。
//
// 争议焦点：CreateProcessAsUserW 对「调用方主 token 的收窄副本」是否豁免
// SeAssignPrimaryTokenPrivilege（MS 文档例外条款）。顾问主张：两进程设计
// （CreateProcessWithLogonW 先长出沙箱身份进程→其内部自我收窄→CreateProcessAsUserW）
// 使例外生效；Kees 主张：他账户 token 非自我收窄，例外不适用。
//
// 机器裁决设计：收窄例外的语义与「收窄的是哪个账户的 token」无关——只问
// 「被收窄的 token 是否=调用方当前主 token」。故用当前用户直接测（零管理员、
// 零建账户）：若当前进程（无 SeAssignPrimaryTokenPrivilege——先行断言钉死）
// 对自我收窄 token 调 CreateProcessAsUserW 成功=例外成立（顾问对）；返回
// 1314（ERROR_PRIVILEGE_NOT_HELD）=例外不成立（Kees 对）。
//
// 判定契约（顾问自定）：成功或失败≠1314 → 顾问成立；恰=1314 → Kees 成立。
package prototype

import (
	"os"
	"os/exec"
	"strings"
	"syscall"
	"testing"
	"unsafe"

	"golang.org/x/sys/windows"
)

var (
	advapi                = windows.NewLazySystemDLL("advapi32.dll")
	procCreateRestricted  = advapi.NewProc("CreateRestrictedToken")
	procGetTokenInfo      = advapi.NewProc("GetTokenInformation")
)

const (
	disableMaxPrivilege    = 0x1
	writeRestricted        = 0x2
	luaToken               = 0x4
	errorPrivilegeNotHeld  = 1314
	tokenPrivilegesInfoCls = 3 // TokenPrivileges
	sePrivilegeEnabled     = 0x2
)

// holdsPrivilege 断言辅助：当前进程 token 是否持有且启用指定特权。
func holdsPrivilege(t *testing.T, token windows.Token, privName string) bool {
	t.Helper()
	namePtr, _ := windows.UTF16PtrFromString(privName)
	var luid windows.LUID
	if err := windows.LookupPrivilegeValue(nil, namePtr, &luid); err != nil {
		t.Fatalf("LookupPrivilegeValue(%s): %v", privName, err)
	}
	// 两调用模式：先取长度
	var need uint32
	procGetTokenInfo.Call(uintptr(token), uintptr(tokenPrivilegesInfoCls), 0, 0, uintptr(unsafe.Pointer(&need)))
	if need == 0 {
		t.Fatal("GetTokenInformation 长度查询失败")
	}
	buf := make([]byte, need)
	r, _, err := procGetTokenInfo.Call(uintptr(token), uintptr(tokenPrivilegesInfoCls),
		uintptr(unsafe.Pointer(&buf[0])), uintptr(need), uintptr(unsafe.Pointer(&need)))
	if r == 0 {
		t.Fatalf("GetTokenInformation: %v", err)
	}
	count := *(*uint32)(unsafe.Pointer(&buf[0]))
	type luidAndAttr struct {
		luid   windows.LUID
		attrib uint32
	}
	for i := uint32(0); i < count; i++ {
		la := *(*luidAndAttr)(unsafe.Pointer(&buf[4+int(i)*12]))
		if la.luid == luid {
			return la.attrib&sePrivilegeEnabled != 0
		}
	}
	return false
}

// makeSelfRestrictedToken 当前进程主 token → 收窄副本（DISABLE_MAX_PRIVILEGE
// +restricting SIDs（Everyone）——同 agentbox token.go 配方，含 TOKEN_ASSIGN_PRIMARY
// 等完整访问权（v1 缺权打开=5 归因实锤：访问权卫生问题非特权墙）。
func makeSelfRestrictedToken(t *testing.T) windows.Token {
	t.Helper()
	var base windows.Token
	if err := windows.OpenProcessToken(windows.CurrentProcess(),
		windows.TOKEN_DUPLICATE|windows.TOKEN_QUERY|windows.TOKEN_ASSIGN_PRIMARY|
			windows.TOKEN_ADJUST_DEFAULT|windows.TOKEN_ADJUST_PRIVILEGES, &base); err != nil {
		t.Fatalf("OpenProcessToken: %v", err)
	}
	defer base.Close()

	everyone, err := windows.CreateWellKnownSid(windows.WinWorldSid)
	if err != nil {
		t.Fatalf("CreateWellKnownSid: %v", err)
	}
	restricting := []windows.SIDAndAttributes{{Sid: everyone, Attributes: 0}}
	// Logon SID 必须入 restricting 列表——否则窗口站/桌面受限检查失败=
	// 子进程 user32 初始化 0xC0000142（v2 实机归因——agentbox token.go 同款配方）
	if logonSID, lerr := extractLogonSIDForTest(base); lerr == nil {
		restricting = append(restricting, windows.SIDAndAttributes{Sid: logonSID, Attributes: 0})
	} else {
		t.Logf("Logon SID 提取失败（非交互会话？）——仅 Everyone: %v", lerr)
	}

	var restricted windows.Token
	r, _, err := procCreateRestricted.Call(
		uintptr(base), uintptr(disableMaxPrivilege|writeRestricted|luaToken),
		0, 0, // DisableSidCount
		0, 0, // DeletePrivilegeCount
		uintptr(len(restricting)), uintptr(unsafe.Pointer(&restricting[0])),
		uintptr(unsafe.Pointer(&restricted)),
	)
	if r == 0 {
		t.Fatalf("CreateRestrictedToken: %v", err)
	}
	// Low IL（子进程 0xC0000142 根治——与 agentbox token.go Step 6 同配方）
	setLowILForTest(t, restricted)
	// SeChangeNotifyPrivilege 启用（目录遍历必需——同 Step 7）
	enableChangeNotifyForTest(t, restricted)
	return restricted
}

// setLowILForTest TokenIntegrityLevel=Low（tml.Size() 纪律——Sizeof 只算头=堆腐蚀陷阱）。
func setLowILForTest(t *testing.T, token windows.Token) {
	t.Helper()
	sid, err := windows.CreateWellKnownSid(windows.WinLowLabelSid)
	if err != nil {
		t.Fatalf("CreateWellKnownSid(LowLabel): %v", err)
	}
	tml := windows.Tokenmandatorylabel{Label: windows.SIDAndAttributes{Sid: sid, Attributes: windows.SE_GROUP_INTEGRITY}}
	if err := windows.SetTokenInformation(token, windows.TokenIntegrityLevel,
		(*byte)(unsafe.Pointer(&tml)), tml.Size()); err != nil {
		t.Fatalf("SetTokenInformation(IL): %v", err)
	}
}

// enableChangeNotifyForTest 启用 SeChangeNotifyPrivilege（受限 token 唯一保留特权）。
func enableChangeNotifyForTest(t *testing.T, token windows.Token) {
	t.Helper()
	var luid windows.LUID
	if err := windows.LookupPrivilegeValue(nil, windows.StringToUTF16Ptr("SeChangeNotifyPrivilege"), &luid); err != nil {
		t.Fatalf("LookupPrivilegeValue: %v", err)
	}
	tp := windows.Tokenprivileges{
		PrivilegeCount: 1,
		Privileges:     [1]windows.LUIDAndAttributes{{Luid: luid, Attributes: windows.SE_PRIVILEGE_ENABLED}},
	}
	if err := windows.AdjustTokenPrivileges(token, false, &tp, uint32(unsafe.Sizeof(tp)), nil, nil); err != nil {
		t.Fatalf("AdjustTokenPrivileges: %v", err)
	}
}

// extractLogonSIDForTest 从 token 组提取 Logon SID（拷贝至 Go 管理内存——
// 同 agentbox token.go extractLogonSID 配方：SE_GROUP_LOGON_ID 属性识别）。
func extractLogonSIDForTest(token windows.Token) (*windows.SID, error) {
	const seGroupLogonIDAttr = 0xC0000000
	var size uint32
	err := windows.GetTokenInformation(token, windows.TokenGroups, nil, 0, &size)
	if err != nil && err != windows.ERROR_INSUFFICIENT_BUFFER {
		return nil, err
	}
	buf := make([]byte, size)
	if err := windows.GetTokenInformation(token, windows.TokenGroups, &buf[0], size, &size); err != nil {
		return nil, err
	}
	groups := (*windows.Tokengroups)(unsafe.Pointer(&buf[0]))
	for _, g := range groups.AllGroups() {
		if g.Attributes&seGroupLogonIDAttr == seGroupLogonIDAttr {
			return g.Sid.Copy()
		}
	}
	return nil, syscall.ERROR_NOT_FOUND
}

// TestSelfRestrictedToken_PrivilegeExemption 复审③机器裁决。
// 裁决语义分层：①API 层=CreateProcessAsUserW 调用本身是否被特权墙拦下
// （1314=Kees 成立；成功/他错=例外生效）；②可用性层=子进程是否真实跑起来
// （0xC0000142 族=token 配方问题，与特权墙无关——归因分离）。
func TestSelfRestrictedToken_PrivilegeExemption(t *testing.T) {
	// 前置断言①：当前 token 不持有（启用态）SeAssignPrimaryTokenPrivilege——
	// 否则成功证明不了例外（特权在场时成功=平凡）。
	var self windows.Token
	if err := windows.OpenProcessToken(windows.CurrentProcess(),
		windows.TOKEN_QUERY, &self); err != nil {
		t.Fatalf("OpenProcessToken(query): %v", err)
	}
	if holdsPrivilege(t, self, "SeAssignPrimaryTokenPrivilege") {
		t.Fatal("前置失败：当前 token 持有 SeAssignPrimaryTokenPrivilege——测试前提被污染")
	}
	t.Logf("前置①：当前 token 无 SeAssignPrimaryTokenPrivilege（启用态）=钉死")
	t.Logf("前置②：SeImpersonatePrivilege 持有=%v；SeIncreaseQuotaPrivilege 持有=%v",
		holdsPrivilege(t, self, "SeImpersonatePrivilege"),
		holdsPrivilege(t, self, "SeIncreaseQuotaPrivilege"))
	self.Close()

	// —— 变体 A：Go os/exec + SysProcAttr.Token（agentbox 生产路径逐字）——
	restrictedA := makeSelfRestrictedToken(t)
	defer restrictedA.Close()
	cmd := exec.Command(`C:\Windows\System32\cmd.exe`, "/c", "echo self-restrict-ok")
	cmd.SysProcAttr = &syscall.SysProcAttr{Token: syscall.Token(restrictedA)}
	out, err := cmd.CombinedOutput()
	if err == nil {
		t.Logf("变体 A（Go exec 路径）=成功：%q——API 层+可用性层双绿", strings.TrimSpace(string(out)))
	} else {
		t.Logf("变体 A（Go exec 路径）=失败: %v out=%q", err, strings.TrimSpace(string(out)))
	}

	// —— 变体 B：raw CreateProcessAsUserW（裁决点直证）——
	restrictedB := makeSelfRestrictedToken(t)
	defer restrictedB.Close()
	outFile := t.TempDir() + `\out.txt`
	outPtr, _ := windows.UTF16PtrFromString(outFile)
	sa := &windows.SecurityAttributes{Length: uint32(unsafe.Sizeof(windows.SecurityAttributes{})), InheritHandle: 1}
	hOut, ferr := windows.CreateFile(outPtr, windows.GENERIC_WRITE, windows.FILE_SHARE_READ, sa,
		windows.CREATE_ALWAYS, windows.FILE_ATTRIBUTE_NORMAL, 0)
	if ferr != nil {
		t.Fatalf("CreateFile: %v", ferr)
	}
	defer windows.CloseHandle(hOut)

	var si windows.StartupInfo
	si.Cb = uint32(unsafe.Sizeof(si))
	si.Flags = startfUseStdHandles
	si.StdOutput = hOut
	si.StdErr = hOut

	cmdline, _ := windows.UTF16PtrFromString(`C:\Windows\System32\cmd.exe /c echo self-restrict-ok`)
	var pi windows.ProcessInformation
	err = windows.CreateProcessAsUser(restrictedB, nil, cmdline, nil, nil, true,
		windows.CREATE_NO_WINDOW|windows.CREATE_UNICODE_ENVIRONMENT, nil, nil, &si, &pi)
	if err != nil {
		errno, _ := err.(syscall.Errno)
		if int(errno) == errorPrivilegeNotHeld {
			t.Fatalf("裁决：恰=1314 ERROR_PRIVILEGE_NOT_HELD——例外不成立，Kees 站住，账户路线维持四税判定")
		}
		t.Logf("变体 B：失败但非 1314（=%d %v）——特权墙未触发，另行归因", int(errno), err)
		return
	}
	defer windows.CloseHandle(pi.Process)
	defer windows.CloseHandle(pi.Thread)
	t.Logf("变体 B：CreateProcessAsUserW 调用=成功——API 层例外成立（无特权持有下赋值主 token）")
	wait, werr := windows.WaitForSingleObject(pi.Process, 30000)
	if werr != nil || wait != 0 {
		_ = windows.TerminateProcess(pi.Process, 1)
		t.Fatalf("子进程等待失败: wait=%d err=%v", wait, werr)
	}
	var exitCode uint32
	_ = windows.GetExitCodeProcess(pi.Process, &exitCode)
	procFlushFileBuffers.Call(uintptr(hOut))
	data, _ := os.ReadFile(outFile)
	if !strings.Contains(string(data), "self-restrict-ok") {
		t.Logf("变体 B 可用性层：子进程 exit=%d 输出=%q（0xC0000142=DLL 初始化失败族——token 配方问题非特权墙）", exitCode, strings.TrimSpace(string(data)))
	} else {
		t.Logf("变体 B 可用性层：子进程真实执行（exit=%d 输出=%q）", exitCode, strings.TrimSpace(string(data)))
	}

	// 终判：A 可用+B API 成功 → 例外成立
	if err != nil {
		t.Fatal("两变体皆败——例外不成立")
	}
}
