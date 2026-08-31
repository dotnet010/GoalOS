//go:build prototype && windows

// appcontainer_fix_windows_test.go——顾问复审隐藏问题①②的修复机制机器裁决
// （2026-08-31）：
//
//  ①稳定 capability SID 共享授予：工具链只读授权不随 session——授予一次稳定
//    SID 长期有效；session 容器通过 SECURITY_CAPABILITIES.Capabilities[] 携带
//    该稳定 SID=被授予面生效。裁决：能力 SID 是否真的被 AC token 访问检查承认
//    （连带对照：无能力 SID 时同路径=拒——授予是因不是巧合）。
//
//  ②沙箱内回环转发链：AC 内监听器（S1/S2 已证同 AC loopback 通）→ 命名管道
//    （FD3 已证 AC 可连宿主管道）→ 宿主中继 → 宿主服务——Ollama 场景的透明
//    承接形态全链实证（消灭「直连静默丢包=秒级超时」延迟坑——隐藏问题②）。
package prototype

import (
	"io"
	"net"
	"os"
	"os/exec"
	"strings"
	"testing"
	"time"
	"unsafe"

	"golang.org/x/sys/windows"
)

// runInAppContainerCaps 带能力 SID 的容器内运行（Cap→访问检查承认面=本测试裁决点）。
// currentDir 显式指定子进程 CWD（实机发现：CWD 继承自 daemon 时若不可读=「当前目录无效」——
// 生产 Provider 必须显式设定 CWD=已授予目录）。
func runInAppContainerCaps(t *testing.T, sid *windows.SID, caps []windows.SIDAndAttributes, cmdline, currentDir string, timeout time.Duration) (int, string) {
	t.Helper()

	outFile := spawnOutPath(t, "out.txt")
	outPathPtr, _ := windows.UTF16PtrFromString(outFile)
	sa := &windows.SecurityAttributes{Length: uint32(unsafe.Sizeof(windows.SecurityAttributes{})), InheritHandle: 1}
	hOut, err := windows.CreateFile(outPathPtr, windows.GENERIC_WRITE, windows.FILE_SHARE_READ, sa, windows.CREATE_ALWAYS, windows.FILE_ATTRIBUTE_NORMAL, 0)
	if err != nil {
		t.Fatalf("CreateFile 捕获文件: %v", err)
	}
	defer windows.CloseHandle(hOut)

	secCaps := securityCapabilities{appContainerSid: sid}
	if len(caps) > 0 {
		secCaps.capabilities = uintptr(unsafe.Pointer(&caps[0]))
		secCaps.capabilityCount = uint32(len(caps))
	}
	attrList, err := windows.NewProcThreadAttributeList(1)
	if err != nil {
		t.Fatalf("NewProcThreadAttributeList: %v", err)
	}
	defer attrList.Delete()
	if err := attrList.Update(procThreadAttributeSecurityCapabilities, unsafe.Pointer(&secCaps), unsafe.Sizeof(secCaps)); err != nil {
		t.Fatalf("UpdateProcThreadAttribute SECURITY_CAPABILITIES: %v", err)
	}

	var siex windows.StartupInfoEx
	siex.Cb = uint32(unsafe.Sizeof(siex))
	siex.Flags = startfUseStdHandles
	siex.StdOutput = hOut
	siex.StdErr = hOut
	siex.ProcThreadAttributeList = attrList.List()

	cmdPtr, _ := windows.UTF16PtrFromString(cmdline)
	var cwdPtr *uint16
	if currentDir != "" {
		cwdPtr, _ = windows.UTF16PtrFromString(currentDir)
	}
	var pi windows.ProcessInformation
	err = windows.CreateProcess(nil, cmdPtr, nil, nil, true,
		windows.CREATE_SUSPENDED|extendedStartupinfoPresent|windows.CREATE_UNICODE_ENVIRONMENT,
		nil, cwdPtr, &siex.StartupInfo, &pi)
	if err != nil {
		t.Fatalf("CreateProcess(AppContainer caps): %v", err)
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
	_ = windows.GetExitCodeProcess(pi.Process, &code)
	procFlushFileBuffers.Call(uintptr(hOut))
	return int(code), readSpawnOut(outFile)
}

// TestAppContainer_SharedCapabilityGrant 隐藏问题①修复机制裁决。
// 关键实机发现（v1 先红）：SECURITY_CAPABILITIES.Capabilities[] 拒收 S-1-15-2-*
// 包 SID（87 参数错）——能力 SID 必须用 DeriveCapabilitySidsFromName 派生的
// S-1-15-3-* 族。附带红利：能力 SID 纯名称派生，无 profile 实体=零残留面
// （连 R-1661 sweep 都不需要覆盖它）。
func TestAppContainer_SharedCapabilityGrant(t *testing.T) {
	const sessProfile = "GoalOS-Spike-AC-Sess"
	deleteAppContainer(sessProfile)
	sidA := createAppContainer(t, sessProfile)
	defer deleteAppContainer(sessProfile)
	defer windows.FreeSid(sidA)
	sidAStr := sidString(t, sidA)

	// 稳定能力 SID：名称派生（确定性、无注册表实体、跨 session 恒定）
	sidR := deriveCapabilitySID(t, "GoalOS-Sandbox-RX")
	sidRStr := sidString(t, sidR)
	t.Logf("session SID=%s 稳定能力 SID=%s", sidAStr, sidRStr)
	if !strings.HasPrefix(sidRStr, "S-1-15-3-") {
		t.Fatalf("能力 SID 族校验：应为 S-1-15-3-* 族: %s", sidRStr)
	}

	// 工具目录：只授予稳定 RX SID（不授予 session SID——隔离变量）
	base := t.TempDir()
	toolDir := base + `\tool`
	toolExe := buildDynamicBinary(t, toolDir, "tool")
	toolData := toolDir + `\data.txt`
	if err := os.WriteFile(toolData, []byte("payload"), 0644); err != nil {
		t.Fatal(err)
	}
	icaclsGrant(t, sidRStr, toolDir, "(OI)(CI)(RX)", false)
	defer icaclsGrant(t, sidRStr, toolDir, "", true)

	// —— 对照组：无能力 SID → 读工具数据=拒（授予是因不是巧合）——
	code, out := runInAppContainer(t, sidA, `"`+toolExe+`" -readin "`+toolData+`"`, 30*time.Second)
	if code == 0 {
		t.Fatalf("对照组 CRITICAL：无能力 SID 竟可读——授予非因，测试设计失效")
	}
	t.Logf("对照组：无能力 SID 读工具数据=拒（授予是因——变量隔离成立）")

	// —— 实验组：Capabilities=[稳定RX SID] → 读通；写仍拒（RX 不扩面）——
	caps := []windows.SIDAndAttributes{{Sid: sidR, Attributes: 4}} // SE_GROUP_ENABLED
	code, out = runInAppContainerCaps(t, sidA, caps, `"`+toolExe+`" -readin "`+toolData+`"`, toolDir, 30*time.Second)
	if code != 0 || !strings.Contains(out, "READ-OK") {
		t.Fatalf("实验组：能力 SID 携带后读工具数据仍失败——稳定共享授予机制不成立: exit=%d %q", code, out)
	}
	t.Logf("实验组：能力 SID 读工具数据=通（%q）——稳定 SID 一次授予长期有效机制成立", strings.TrimSpace(out))
	code, out = runInAppContainerCaps(t, sidA, caps, `"`+toolExe+`" -writeout "`+toolDir+`\w.txt"`, toolDir, 30*time.Second)
	if code == 0 || !strings.Contains(out, "WRITE-DENIED") {
		t.Fatalf("实验组 CRITICAL：RX 授予下写工具目录竟成功——授予扩面: exit=%d %q", code, out)
	}
	t.Logf("实验组：写工具目录=拒（RX 不扩面——写面仍 session 粒度）")
}

// forwarderSource 沙箱内回环转发器：loopback 监听 → 命名管道 → 宿主中继。
// 管道客户端=原始 syscall.CreateFile/ReadFile/WriteFile（v1 先红：os.OpenFile
// 开管道句柄经 Go os.File 层语义不合——宿主对宿主同断，非 AC 特异性；stdlib-only
// 约束：子二进制独立构建无 vendor 可用 x/sys）。
const forwarderSource = `package main

import (
	"fmt"
	"net"
	"os"
	"syscall"
)

func main() {
	ln, err := net.Listen("tcp", os.Args[1])
	if err != nil {
		fmt.Println("FWD-LISTEN-FAIL: " + err.Error())
		os.Exit(2)
	}
	fmt.Println("FWD-LISTENING")
	conn, err := ln.Accept()
	if err != nil {
		fmt.Println("FWD-ACCEPT-FAIL: " + err.Error())
		os.Exit(3)
	}
	fmt.Println("FWD-ACCEPTED")
	namePtr, _ := syscall.UTF16PtrFromString(os.Args[2])
	hPipe, err := syscall.CreateFile(namePtr, syscall.GENERIC_READ|syscall.GENERIC_WRITE,
		0, nil, syscall.OPEN_EXISTING, 0, 0)
	if err != nil {
		fmt.Println("FWD-PIPE-FAIL: " + err.Error())
		os.Exit(4)
	}
	fmt.Println("FWD-PIPE-OPEN")
	// 请求-响应顺序模式（与 TestPipeIso 已证形态同构——全双工并发不在本裁决范围）
	buf := make([]byte, 4096)
	n, err := conn.Read(buf)
	if err != nil || n == 0 {
		fmt.Printf("FWD-C2P-READFAIL n=%d err=%v\n", n, err)
		os.Exit(5)
	}
	fmt.Printf("FWD-C2P-READ n=%d\n", n)
	var written uint32
	if werr := syscall.WriteFile(hPipe, buf[:n], &written, nil); werr != nil {
		fmt.Println("FWD-C2P-WRITEFAIL: " + werr.Error())
		os.Exit(6)
	}
	fmt.Printf("FWD-C2P-WROTE n=%d\n", written)
	var rn uint32
	if rerr := syscall.ReadFile(hPipe, buf, &rn, nil); rerr != nil || rn == 0 {
		fmt.Printf("FWD-P2C-READFAIL n=%d err=%v\n", rn, rerr)
		os.Exit(7)
	}
	if _, werr := conn.Write(buf[:rn]); werr != nil {
		fmt.Println("FWD-P2C-WRITEFAIL: " + werr.Error())
		os.Exit(8)
	}
	fmt.Println("FWD-RELAYED")
}
`

// fwdClientSource 沙箱内客户端：直连回环转发口（模拟「SDK 默认直连 localhost」形态）。
const fwdClientSource = `package main

import (
	"fmt"
	"net"
	"os"
	"time"
)

func main() {
	conn, err := net.DialTimeout("tcp", os.Args[1], 3*time.Second)
	if err != nil {
		fmt.Println("CLIENT-DIAL-FAIL: " + err.Error())
		os.Exit(2)
	}
	defer conn.Close()
	conn.SetDeadline(time.Now().Add(8 * time.Second))
	conn.Write([]byte("ping-ac"))
	buf := make([]byte, 64)
	n, err := conn.Read(buf)
	if err != nil {
		fmt.Println("CLIENT-READ-FAIL: " + err.Error())
		os.Exit(3)
	}
	fmt.Println("FWD-ECHO: " + string(buf[:n]))
}
`

var procSDDLToSD = advapi.NewProc("ConvertStringSecurityDescriptorToSecurityDescriptorW")

// deriveCapabilitySID DeriveCapabilitySidsFromName 派生稳定能力 SID（S-1-15-3-* 族——
// 纯名称派生无 profile 实体；kernelbase.dll 导出——kernel32/apiset DLL 均无此导出
// （v1/v2 先红确认归属；v3 签名实为五参（SID 数组+计数输出）——三参调用=0xC0000005
// 实机实锤，先红修正）。
func deriveCapabilitySID(t *testing.T, capName string) *windows.SID {
	t.Helper()
	capdll := windows.NewLazySystemDLL("kernelbase.dll")
	proc := capdll.NewProc("DeriveCapabilitySidsFromName")
	namePtr, _ := windows.UTF16PtrFromString(capName)
	var groupSids, capSids *windows.SID // PSID* 数组输出
	var groupCount, capCount uint32
	r1, _, err := proc.Call(
		uintptr(unsafe.Pointer(namePtr)),
		uintptr(unsafe.Pointer(&groupSids)), uintptr(unsafe.Pointer(&groupCount)),
		uintptr(unsafe.Pointer(&capSids)), uintptr(unsafe.Pointer(&capCount)),
	)
	if r1 == 0 {
		t.Fatalf("DeriveCapabilitySidsFromName: %v", err)
	}
	if capCount == 0 || capSids == nil {
		t.Fatal("能力 SID 数组为空")
	}
	defer windows.LocalFree(windows.Handle(unsafe.Pointer(groupSids)))
	// 拷贝首个能力 SID 到 Go 管理内存后释放数组
	first := *(**windows.SID)(unsafe.Pointer(capSids))
	copied, copyErr := first.Copy()
	windows.LocalFree(windows.Handle(unsafe.Pointer(capSids)))
	if copyErr != nil {
		t.Fatalf("能力 SID 拷贝: %v", copyErr)
	}
	return copied
}

// TestAppContainer_LoopbackForwarder 隐藏问题②解法裁决——全链：
// AC 客户端→AC 转发器（loopback）→命名管道→宿主中继→宿主 echo 服务→原路返回。
func TestAppContainer_LoopbackForwarder(t *testing.T) {
	const profile = "GoalOS-Spike-AC-Fwd"
	deleteAppContainer(profile)
	sid := createAppContainer(t, profile)
	defer deleteAppContainer(profile)
	defer windows.FreeSid(sid)

	base := t.TempDir()
	fwd := buildBinary(t, base, "forwarder", forwarderSource)
	client := buildBinary(t, base, "fwdclient", fwdClientSource)

	// 宿主 echo 服务（模拟宿主侧 Ollama 族）
	echoLn, err := net.Listen("tcp", "127.0.0.1:18751")
	if err != nil {
		t.Fatalf("echo 监听: %v", err)
	}
	defer echoLn.Close()
	go func() {
		for {
			c, err := echoLn.Accept()
			if err != nil {
				return
			}
			go io.Copy(c, c)
		}
	}()

	// 宿主命名管道服务（SDDL 放开 Everyone+ALL APPLICATION PACKAGES(AC)——
	// v1 先红：仅 Everyone=AC 被拒（Access denied——与文件语义同构：AC 检查
	// 需包/能力/AAP 授权，Everyone 不满足）；中继到 echo；CancelIoEx 兜底防挂死。
	const pipeName = `\\.\pipe\GoalOS-Fwd-Test-2026`
	sddlPtr, _ := windows.UTF16PtrFromString("D:P(A;;GA;;;WD)(A;;GA;;;AC)")
	var sd *windows.SECURITY_DESCRIPTOR
	r1, _, err := procSDDLToSD.Call(uintptr(unsafe.Pointer(sddlPtr)), 1, uintptr(unsafe.Pointer(&sd)), 0)
	if r1 == 0 {
		t.Fatalf("SDDL→SD: %v", err)
	}
	defer windows.LocalFree(windows.Handle(unsafe.Pointer(sd)))
	sa := &windows.SecurityAttributes{Length: uint32(unsafe.Sizeof(windows.SecurityAttributes{})), SecurityDescriptor: sd}
	pipePtr, _ := windows.UTF16PtrFromString(pipeName)
	hPipe, err := windows.CreateNamedPipe(pipePtr, windows.PIPE_ACCESS_DUPLEX,
		windows.PIPE_TYPE_BYTE|windows.PIPE_READMODE_BYTE|windows.PIPE_WAIT,
		1, 4096, 4096, 0, sa)
	if err != nil {
		t.Fatalf("CreateNamedPipe: %v", err)
	}
	t.Cleanup(func() {
		procCancelIoEx := kern32.NewProc("CancelIoEx")
		procCancelIoEx.Call(uintptr(hPipe), 0) // 解除 ConnectNamedPipe 阻塞防挂死
		windows.CloseHandle(hPipe)
	})
	go func() {
		// 阻塞等 AC 转发器连接，然后与 echo 服务双向中继
		if err := windows.ConnectNamedPipe(hPipe, nil); err != nil {
			t.Logf("relay: ConnectNamedPipe 失败: %v", err)
			return
		}
		t.Logf("relay: 管道已连接")
		backend, err := net.DialTimeout("tcp", "127.0.0.1:18751", 3*time.Second)
		if err != nil {
			t.Logf("relay: 后端 echo 拨号失败: %v", err)
			return
		}
		t.Logf("relay: 后端 echo 已连")
		defer backend.Close()
		// 请求-响应顺序中继（读请求→写后端→读响应→写管道——与 PipeIso 已证形态同构）
		buf := make([]byte, 4096)
		var n uint32
		if err := windows.ReadFile(hPipe, buf, &n, nil); err != nil || n == 0 {
			t.Logf("relay: 管道读失败 n=%d: %v", n, err)
			return
		}
		t.Logf("relay: 收到请求 %d 字节", n)
		if _, err := backend.Write(buf[:n]); err != nil {
			t.Logf("relay: 后端写失败: %v", err)
			return
		}
		backend.SetReadDeadline(time.Now().Add(5 * time.Second))
		rn, err := backend.Read(buf)
		if err != nil || rn == 0 {
			t.Logf("relay: 后端读失败 n=%d: %v", rn, err)
			return
		}
		var written uint32
		if err := windows.WriteFile(hPipe, buf[:rn], &written, nil); err != nil {
			t.Logf("relay: 管道写失败: %v", err)
			return
		}
		t.Logf("relay: 响应已回写 %d 字节", written)
	}()

	// AC 二分锚点：GOALOS_FWD_PLAIN=1 时转发器+客户端全走普通进程（隔离 AC 特异性）
	plainMode := os.Getenv("GOALOS_FWD_PLAIN") == "1"
	fwdOut := base + `\fwd.out`
	var pi *windows.ProcessInformation
	if plainMode {
		f, _ := os.Create(fwdOut)
		t.Cleanup(func() { f.Close() })
		cmd := exec.Command(fwd, "127.0.0.1:18750", pipeName)
		cmd.Stdout = f
		cmd.Stderr = f
		if err := cmd.Start(); err != nil {
			t.Fatalf("plain forwarder 启动: %v", err)
		}
		t.Cleanup(func() { cmd.Process.Kill(); cmd.Wait() })
	} else {
		pi = spawnInAppContainer(t, sid, `"`+fwd+`" 127.0.0.1:18750 `+pipeName, fwdOut)
		defer killAndWait(t, pi)
	}
	time.Sleep(2 * time.Second) // 等监听+管道连接

	// AC 客户端直连回环转发口（SDK 默认直连形态——无感知被承接）
	var code int
	var out string
	if plainMode {
		cbytes, cerr := exec.Command(client, "127.0.0.1:18750").CombinedOutput()
		out = string(cbytes)
		if cerr == nil {
			code = 0
		} else {
			code = 1
		}
	} else {
		code, out = runInAppContainer(t, sid, `"`+client+`" 127.0.0.1:18750`, 30*time.Second)
	}
	if code != 0 || !strings.Contains(out, "FWD-ECHO: ping-ac") {
		if pi != nil {
			killAndWait(t, pi)
		}
		t.Fatalf("全链失败: exit=%d out=%q fwd=%q", code, strings.TrimSpace(out), strings.TrimSpace(readSpawnOut(fwdOut)))
	}
	t.Logf("全链=通（%q）——沙箱内 loopback 转发承接宿主服务机制成立（Ollama 场景解法实证）", strings.TrimSpace(out))
	if pi != nil {
		killAndWait(t, pi)
	}
	t.Logf("转发器侧输出=%q", strings.TrimSpace(readSpawnOut(fwdOut)))
}