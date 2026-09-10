//go:build windows && prototype

// winlogon_windows_test.go——S-266-04 spike：CreateProcessWithLogonW 零特权性实机补证
//（会议 #262 复审(3)遗留——账户路线三税之一的「CreateProcessWithLogonW 需管理员」
// 存疑税目；顾问裁文献级→本 spike 实机裁决）。
//
// 裁决形态（三重证据链）：
//   A. 管理员态 caller 调 CreateProcessWithLogonW=成功（基线——文献无争议面）；
//   B. 落盘子进程 token=非管理员（TokenElevation 取证——证明子进程真是受限上下文）；
//   C. **非管理员子进程内再次 CreateProcessWithLogonW=成功**（嵌套自登录——
//      若成立=该 API 调用面零特权需求实锤，「需管理员」税目撤销）。
//
// 纪律：setup（建/删临时受限账户）允许 net.exe（非断言路径）；断言路径=纯 Win32
// syscall（advapi32 LazyDLL 直调——探针纪律 R-1666 同构）。
// 运行面：goalos-test-win（Win11 家庭版 26200，ssh 会话=Administrators 启用）。
package prototype

import (
	"fmt"
	"os"
	"os/exec"
	"strings"
	"testing"
	"time"
	"unicode/utf16"
	"unsafe"

	"golang.org/x/sys/windows"
)

var procCreateProcessWithLogonW = windows.NewLazySystemDLL("advapi32.dll").NewProc("CreateProcessWithLogonW")

// cWDPublicPath 子进程 CWD（C:\Users\Public——跨账户可读可写；继承父 CWD=新账户
// 私有 home 不可读=子进程静默死，F6 同族教训）。
const cWDPublicPath = `C:\Users\Public`

// createProcessWithLogon 纯 syscall 调用（11 参——advapi32）。
// logonFlags: 0=默认（LOGON32_LOGON_INTERACTIVE，不载 profile）；
// 1=LOGON_WITH_PROFILE（载用户 profile 蜂巢——实机实锤：载蜂巢后环境块按注册表
// 重建，调用方继承丢失→故 extraEnv 显式构造环境块传入）。
// cwd 显式传（F6 教训：继承父 CWD=新账户可能不可读=子进程静默死）。
// 另两实机坑：CREATE_NO_WINDOW 必需（session 0 无交互 window station——缺则子进程
// 0xC0000142 静默死）；image 须新账户可读（私有 home=Access denied）。
func createProcessWithLogon(user, domain, password, cmdline string, logonFlags uint32, cwd string, extraEnv []string) error {
	u, _ := windows.UTF16PtrFromString(user)
	d, _ := windows.UTF16PtrFromString(domain)
	p, _ := windows.UTF16PtrFromString(password)
	c, _ := windows.UTF16PtrFromString(cmdline)
	w, _ := windows.UTF16PtrFromString(cwd)
	var envPtr *uint16
	if extraEnv != nil {
		// 显式环境块（UTF16 双 NUL 终止——os.Environ+追加项）
		all := append(os.Environ(), extraEnv...)
		var u16 []uint16
		for _, e := range all {
			u16 = append(u16, utf16.Encode([]rune(e))...)
			u16 = append(u16, 0)
		}
		u16 = append(u16, 0)
		envPtr = &u16[0]
	}
	var si windows.StartupInfo
	si.Cb = uint32(unsafe.Sizeof(si))
	var pi windows.ProcessInformation
	r1, _, err := procCreateProcessWithLogonW.Call(
		uintptr(unsafe.Pointer(u)), uintptr(unsafe.Pointer(d)), uintptr(unsafe.Pointer(p)),
		uintptr(logonFlags),
		0, // lpApplicationName=NULL——走 cmdline 解析
		uintptr(unsafe.Pointer(c)),
		windows.CREATE_UNICODE_ENVIRONMENT|0x08000000, // CREATE_NO_WINDOW
		uintptr(unsafe.Pointer(envPtr)), uintptr(unsafe.Pointer(w)),
		uintptr(unsafe.Pointer(&si)), uintptr(unsafe.Pointer(&pi)))
	if r1 == 0 {
		return err
	}
	// 诊断面：等子进程退出+取退出码（S-266-04 排障——创建成功≠运行成功）
	windows.WaitForSingleObject(pi.Process, 8000)
	var exitCode uint32
	_ = windows.GetExitCodeProcess(pi.Process, &exitCode)
	fmt.Printf("[winlogon-dbg] child exit=0x%08X\n", exitCode)
	windows.CloseHandle(pi.Process)
	windows.CloseHandle(pi.Thread)
	return nil
}

// TestWinLogonZeroPrivilege S-266-04 主裁决（管理员态运行——setup 建账户需要）。
// 已出数闭环（2026-09-07 goalos-test-win——scripts/red-evidence/2026-09-07-s266-04-
// winlogon-adjudication.txt）；保留=复核通道。环境门禁：非管理员/非授权窗口=SKIP
//（建账户+跨账户拉进程=Defender 启发式敏感面——实机实证 2026-09-07 go.exe 被隔离）。
func TestWinLogonZeroPrivilege(t *testing.T) {
	if os.Getenv("GOALOS_S266_SPIKE") != "1" {
		t.Skip("环境门禁：GOALOS_S266_SPIKE=1 管理员实机窗口（已出数闭环——重跑=复核）")
	}
	user := fmt.Sprintf("glsp-%d", time.Now().UnixNano()%100000000)
	password := fmt.Sprintf("Gl!%08x", time.Now().UnixNano()%0xFFFFFFFF) // 复杂度+≤14 字符（Home 密码策略实证：>14 拒绝）
	self, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	// 映像面：新账户 token 打开 image——私有 home 不可读=Access denied（实机实锤）。
	// 复制到 Public（全域可读执行）再作为子进程目标。
	publicSelf := `C:\Users\Public\` + fmt.Sprintf("s266-%d.exe", time.Now().UnixNano()%100000000)
	data, err := os.ReadFile(self)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(publicSelf, data, 0755); err != nil {
		t.Fatalf("映像复制到 Public 失败: %v", err)
	}
	defer os.Remove(publicSelf)
	self = publicSelf

	// setup：建临时受限账户（net.exe=setup 面非断言面）+defer 清理
	if out, err := exec.Command("net", "user", user, password, "/add").CombinedOutput(); err != nil {
		t.Fatalf("建账户失败: %v out=%s", err, out)
	}
	defer exec.Command("net", "user", user, "/delete").Run()
	t.Logf("临时受限账户=%s（USER_PRIV_USER——非管理员）", user)

	// A：管理员态 caller 基线
	childCmd := fmt.Sprintf(`"%s" --child-mode`, self)
	if err := createProcessWithLogon(user, ".", password, childCmd, 1, cWDPublicPath, nil); err != nil {
		t.Fatalf("A 相：管理员态 CreateProcessWithLogonW 失败: %v", err)
	}
	t.Log("A 相绿：管理员态调用成功（基线）")

	// A2 最小子进程对照：cmd.exe 落盘标记——分离「logon 机制」vs「Go 二进制」变量
	cmdMark := `C:\Users\Public\s266-cmd-` + user + ".mark"
	defer os.Remove(cmdMark)
	if err := createProcessWithLogon(user, ".", password,
		`C:\Windows\System32\cmd.exe /c echo ok> "`+cmdMark+`"`, 1, cWDPublicPath, nil); err != nil {
		t.Fatalf("A2 相：cmd.exe 最小子进程创建失败: %v", err)
	}
	cmdOK := false
	for i := 0; i < 20; i++ {
		if _, err := os.Stat(cmdMark); err == nil {
			cmdOK = true
			break
		}
		time.Sleep(250 * time.Millisecond)
	}
	t.Logf("A2 相：cmd.exe 子进程落盘=%v（false=logon 上下文面坏/非 Go 二进制因素）", cmdOK)

	// B+C：子进程（受限 token）内自证非管理员+嵌套再登录
	// ——child-mode 由 TestMain 拦截（见本文件底部 TestMain）
	// 此处父侧等待标记文件（child 产出证据文件）
	marker := `C:\Users\Public\s266-` + user + ".mark" // Public=跨账户可写（子进程是另一账户——父 temp 私有不可写）
	defer os.Remove(marker)
	// 嵌套裁决凭证经环境变量传入子进程（createProcessWithLogon env=nil=继承父环境——
	// spike 内部面，非生产凭证纪律面）
	os.Setenv("GLSP_USER", user)
	os.Setenv("GLSP_PWD", password)
	defer os.Unsetenv("GLSP_USER")
	defer os.Unsetenv("GLSP_PWD")
	childCmd2 := fmt.Sprintf(`"%s" --child-mode "%s"`, self, marker)
	if err := createProcessWithLogon(user, ".", password, childCmd2, 1, cWDPublicPath, []string{"GLSP_USER=" + user, "GLSP_PWD=" + password}); err != nil {
		t.Fatalf("B 相：创建证据子进程失败: %v", err)
	}
	deadline := time.Now().Add(30 * time.Second)
	for time.Now().Before(deadline) {
		if data, err := os.ReadFile(marker); err == nil {
			t.Logf("子进程证据:\n%s", data)
			if !strings.Contains(string(data), "ELEVATED=0") {
				t.Fatal("B 相红：子进程 token 竟是管理员——嵌套裁决上下文无效")
			}
			if !strings.Contains(string(data), "NESTED_OK=1") {
				t.Fatal("C 相红：非管理员子进程嵌套 CreateProcessWithLogonW 失败——「需管理员」税目成立")
			}
			t.Log("B 相绿：子进程非管理员实锤；C 相绿：嵌套自登录成功——零特权性实锤")
			return
		}
		time.Sleep(300 * time.Millisecond)
	}
	t.Fatal("子进程证据文件 30s 未出现")
}

// TestMain 拦截子进程模式（re-exec 自身——零外部二进制依赖，R-1666 同构）。
func TestMain(m *testing.M) {
	if len(os.Args) >= 2 && os.Args[1] == "--child-mode" {
		childMain()
		return // childMain 不返回
	}
	if len(os.Args) >= 2 && os.Args[1] == "--grandchild-mode" {
		os.Exit(0) // 孙进程=存在即证据（C 相 spawn 成功即可，无需产出）
	}
	// --nest-probe <user> <pwd> <marker>：任务计划程序实验载体——批处理登录会话
	// （非嵌套的非管理员上下文）直调嵌套 CreateProcessWithLogonW，结果落盘。
	// 判别目标：「非管理员不可调」vs「CreateProcessWithLogonW 子进程上下文不可调」。
	if len(os.Args) >= 5 && os.Args[1] == "--nest-probe" {
		self, _ := os.Executable()
		err := createProcessWithLogon(os.Args[2], ".", os.Args[3],
			fmt.Sprintf(`"%s" --grandchild-mode`, self), 1, cWDPublicPath, nil)
		result := "NEST_PROBE_OK=1"
		if err != nil {
			result = fmt.Sprintf("NEST_PROBE_OK=0 err=%v", err)
		}
		_ = os.WriteFile(os.Args[4], []byte(result), 0600)
		os.Exit(0)
	}
	os.Exit(m.Run())
}

// childMain 受限子进程证据面：(1)TokenElevation=0 自证非管理员 (2)嵌套再调
// CreateProcessWithLogonW（同一受限账户自登录）(3)写证据文件（父侧轮询读取）。
func childMain() {
	var b strings.Builder
	// (1)token 取证（GetTokenInformation TokenElevation——管理员=1）
	var token windows.Token
	if err := windows.OpenProcessToken(windows.CurrentProcess(), windows.TOKEN_QUERY, &token); err == nil {
		var elevation uint32
		var retLen uint32
		if err := windows.GetTokenInformation(token, windows.TokenElevation,
			(*byte)(unsafe.Pointer(&elevation)), 4, &retLen); err == nil {
			fmt.Fprintf(&b, "ELEVATED=%d\n", elevation)
		}
		token.Close()
	}
	// (2)嵌套自登录（C 相——本进程已是非管理员上下文）
	self, _ := os.Executable()
	nestedCmd := fmt.Sprintf(`"%s" --grandchild-mode`, self)
	// 从环境拿账户（父侧经命令行传入——child 命令行 argv[2]=marker，账户名由父写死到环境？）
	// 简化：父侧把账户名/密码经环境变量传入（spike 内部面，非生产凭证纪律面）
	nestedUser := os.Getenv("GLSP_USER")
	nestedPwd := os.Getenv("GLSP_PWD")
	if nestedUser == "" {
		b.WriteString("NESTED_OK=0 (no creds env)\n")
	} else if err := createProcessWithLogon(nestedUser, ".", nestedPwd, nestedCmd, 1, cWDPublicPath, nil); err != nil {
		fmt.Fprintf(&b, "NESTED_OK=0 flags1 err=%v\n", err)
		// 变体：不载 profile 蜂巢（嵌套场景同账户蜂巢已被本进程占用——排除蜂巢冲突变量）
		if err2 := createProcessWithLogon(nestedUser, ".", nestedPwd, nestedCmd, 0, cWDPublicPath, nil); err2 != nil {
			fmt.Fprintf(&b, "NESTED_OK=0 flags0 err=%v\n", err2)
		} else {
			b.WriteString("NESTED_OK=1 flags0（不载蜂巢成功——蜂巢占用是变量）\n")
		}
	} else {
		b.WriteString("NESTED_OK=1\n")
	}
	if len(os.Args) >= 3 {
		_ = os.WriteFile(os.Args[2], []byte(b.String()), 0600)
	} else {
		fmt.Print(b.String()) // A 相路径：无 marker 参数=直接打印（进程一闪而过，父侧不读）
	}
	os.Exit(0)
}
