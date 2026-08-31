//go:build prototype && windows

// appcontainer_fix2_windows_test.go——顾问第五轮两条的机器裁决（2026-08-31）：
//
//  ①具名能力粒度：「GoalOS-Sandbox-RX 一笼统能力管所有工具链」违背 D-1
//    按 Action 编译能力集原则——修正为每工具链一具名能力、session 按声明
//    携带。裁决：携带 cap-node 的 session 读 python 授予目录必须=拒
//    （选择性成立），携带对应能力=通。
//
//  ②双单向管道并发转发（R-1662 根治——替代「一根双向管道两端阻塞读写」
//    的经典死锁形态）：请求/响应各走独立管道句柄，两客户端并发×双向
//    全链——生产并发形态的正面实证（不拿顺序单请求冒充并发）。
package prototype

import (
	"fmt"
	"io"
	"net"
	"os"
	"strings"
	"testing"
	"time"
	"unsafe"

	"golang.org/x/sys/windows"
)

// TestAppContainer_CapabilityGranularity ①具名能力选择性裁决。
func TestAppContainer_CapabilityGranularity(t *testing.T) {
	const sessProfile = "GoalOS-Spike-AC-Gran"
	deleteAppContainer(sessProfile)
	sidA := createAppContainer(t, sessProfile)
	defer deleteAppContainer(sessProfile)
	defer windows.FreeSid(sidA)

	capPy := deriveCapabilitySID(t, "GoalOS-TC-python")
	capNode := deriveCapabilitySID(t, "GoalOS-TC-node")
	capPyStr, capNodeStr := sidString(t, capPy), sidString(t, capNode)
	t.Logf("cap-python=%s\ncap-node  =%s", capPyStr, capNodeStr)

	base := t.TempDir()
	pyDir := base + `\pytool`
	nodeDir := base + `\nodetool`
	pyExe := buildDynamicBinary(t, pyDir, "pytool")
	nodeExe := buildDynamicBinary(t, nodeDir, "nodetool")
	pyData := pyDir + `\data.txt`
	nodeData := nodeDir + `\data.txt`
	os.WriteFile(pyData, []byte("py"), 0644)
	os.WriteFile(nodeData, []byte("nd"), 0644)
	// 各自具名能力各自授予（正交授权面）
	icaclsGrant(t, capPyStr, pyDir, "(OI)(CI)(RX)", false)
	defer icaclsGrant(t, capPyStr, pyDir, "", true)
	icaclsGrant(t, capNodeStr, nodeDir, "(OI)(CI)(RX)", false)
	defer icaclsGrant(t, capNodeStr, nodeDir, "", true)

	// —— 场景 1：session 只携带 cap-node（Action 只声明 node）——
	capsNode := []windows.SIDAndAttributes{{Sid: capNode, Attributes: 4}}
	code, out := runInAppContainerCaps(t, sidA, capsNode, `"`+nodeExe+`" -readin "`+nodeData+`"`, nodeDir, 30*time.Second)
	if code != 0 || !strings.Contains(out, "READ-OK") {
		t.Fatalf("S1：携带 cap-node 读 node 目录失败: exit=%d %q", code, out)
	}
	t.Logf("S1 携带 cap-node 读 node=通")
	code, out = runInAppContainerCaps(t, sidA, capsNode, `"`+pyExe+`" -readin "`+pyData+`"`, pyDir, 30*time.Second)
	if code == 0 {
		t.Fatalf("S1 CRITICAL：只声明 node 的 session 竟可读 python 工具链——能力粒度失守")
	}
	t.Logf("S1 携带 cap-node 读 python=拒（选择性成立——D-1 按声明编译能力集落地）")

	// —— 场景 2：session 携带双能力（Action 声明 python+node）——
	capsBoth := []windows.SIDAndAttributes{{Sid: capNode, Attributes: 4}, {Sid: capPy, Attributes: 4}}
	code, out = runInAppContainerCaps(t, sidA, capsBoth, `"`+pyExe+`" -readin "`+pyData+`"`, pyDir, 30*time.Second)
	if code != 0 || !strings.Contains(out, "READ-OK") {
		t.Fatalf("S2：双能力携带读 python 失败: exit=%d %q", code, out)
	}
	t.Logf("S2 携带双能力读 python=通（按需声明=按需获得）")
}

// fwd2Source 双单向管道转发器（每连接一对独立管道：Req=AC→宿主写向，
// Resp=宿主→AC 读向——根治同步句柄全双工死锁）。
const fwd2Source = `package main

import (
	"fmt"
	"net"
	"os"
	"syscall"
)

func openPipe(name string, access uint32) syscall.Handle {
	p, _ := syscall.UTF16PtrFromString(name)
	h, err := syscall.CreateFile(p, access, 0, nil, syscall.OPEN_EXISTING, 0, 0)
	if err != nil {
		fmt.Println("PIPE-OPEN-FAIL " + name + ": " + err.Error())
		os.Exit(9)
	}
	return h
}

func handleConn(idx int, conn net.Conn, base string) {
	req := openPipe(fmt.Sprintf("\\\\.\\pipe\\%s-Req%d", base, idx), syscall.GENERIC_WRITE)
	resp := openPipe(fmt.Sprintf("\\\\.\\pipe\\%s-Resp%d", base, idx), syscall.GENERIC_READ)
	fmt.Printf("FWD2-PIPES-%d-OPEN\n", idx)
	// conn → Req 管道（独立句柄，写向）
	go func() {
		buf := make([]byte, 4096)
		for {
			n, err := conn.Read(buf)
			if n > 0 {
				var w uint32
				if werr := syscall.WriteFile(req, buf[:n], &w, nil); werr != nil {
					return
				}
			}
			if err != nil {
				return
			}
		}
	}()
	// Resp 管道 → conn（独立句柄，读向）
	buf := make([]byte, 4096)
	for {
		var n uint32
		err := syscall.ReadFile(resp, buf, &n, nil)
		if err != nil || n == 0 {
			return
		}
		if _, werr := conn.Write(buf[:n]); werr != nil {
			return
		}
	}
}

func main() {
	ln, err := net.Listen("tcp", os.Args[1])
	if err != nil {
		fmt.Println("FWD2-LISTEN-FAIL: " + err.Error())
		os.Exit(2)
	}
	fmt.Println("FWD2-LISTENING")
	base := os.Args[2]
	idx := 0
	for {
		conn, err := ln.Accept()
		if err != nil {
			return
		}
		fmt.Printf("FWD2-ACCEPT-%d\n", idx)
		go handleConn(idx, conn, base)
		idx++
	}
}
`

// fwdClient2Source 负载可配的客户端（区分并发连接的响应归属）。
const fwdClient2Source = `package main

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
	conn.Write([]byte(os.Args[2]))
	buf := make([]byte, 64)
	n, err := conn.Read(buf)
	if err != nil {
		fmt.Println("CLIENT-READ-FAIL: " + err.Error())
		os.Exit(3)
	}
	fmt.Println("FWD2-ECHO: " + string(buf[:n]))
}
`

// hostPipePair 宿主侧单连接中继（Req 读向/Resp 写向+独立后端连接）。
func hostPipePair(t *testing.T, base string, idx int, backendAddr string) {
	t.Helper()
	sddlPtr, _ := windows.UTF16PtrFromString("D:P(A;;GA;;;WD)(A;;GA;;;AC)")
	var sd *windows.SECURITY_DESCRIPTOR
	r1, _, err := procSDDLToSD.Call(uintptr(unsafe.Pointer(sddlPtr)), 1, uintptr(unsafe.Pointer(&sd)), 0)
	if r1 == 0 {
		t.Fatalf("SDDL→SD: %v", err)
	}
	defer windows.LocalFree(windows.Handle(unsafe.Pointer(sd)))
	sa := &windows.SecurityAttributes{Length: uint32(unsafe.Sizeof(windows.SecurityAttributes{})), SecurityDescriptor: sd}

	mk := func(suffix string) windows.Handle {
		namePtr, _ := windows.UTF16PtrFromString(fmt.Sprintf(`\\.\pipe\%s-%s%d`, base, suffix, idx))
		h, err := windows.CreateNamedPipe(namePtr, windows.PIPE_ACCESS_DUPLEX,
			windows.PIPE_TYPE_BYTE|windows.PIPE_READMODE_BYTE|windows.PIPE_WAIT,
			1, 4096, 4096, 0, sa)
		if err != nil {
			t.Fatalf("CreateNamedPipe %s%d: %v", suffix, idx, err)
		}
		return h
	}
	reqH, respH := mk("Req"), mk("Resp")
	t.Cleanup(func() {
		procCancel := kern32.NewProc("CancelIoEx")
		procCancel.Call(uintptr(reqH), 0)
		procCancel.Call(uintptr(respH), 0)
		windows.CloseHandle(reqH)
		windows.CloseHandle(respH)
	})

	go func() {
		if err := windows.ConnectNamedPipe(reqH, nil); err != nil {
			t.Logf("relay%d: Req connect: %v", idx, err)
			return
		}
		if err := windows.ConnectNamedPipe(respH, nil); err != nil {
			t.Logf("relay%d: Resp connect: %v", idx, err)
			return
		}
		t.Logf("relay%d: 双管道已连接", idx)
		backend, err := net.DialTimeout("tcp", backendAddr, 3*time.Second)
		if err != nil {
			t.Logf("relay%d: 后端失败: %v", idx, err)
			return
		}
		defer backend.Close()
		// Req 管道 → 后端（唯一写 req 的方向）
		go func() {
			buf := make([]byte, 4096)
			for {
				var n uint32
				err := windows.ReadFile(reqH, buf, &n, nil)
				if err != nil || n == 0 {
					return
				}
				if _, err := backend.Write(buf[:n]); err != nil {
					return
				}
			}
		}()
		// 后端 → Resp 管道（唯一写 resp 的方向）
		buf := make([]byte, 4096)
		for {
			n, err := backend.Read(buf)
			if err != nil || n == 0 {
				return
			}
			var w uint32
			if err := windows.WriteFile(respH, buf[:n], &w, nil); err != nil {
				return
			}
		}
	}()
}

// TestAppContainer_DuplexPipeForwarder ②双单向管道并发根治裁决：
// 两客户端并发×双向全链——响应归属正确（payload 区分）。
func TestAppContainer_DuplexPipeForwarder(t *testing.T) {
	const profile = "GoalOS-Spike-AC-Fwd2"
	deleteAppContainer(profile)
	sid := createAppContainer(t, profile)
	defer deleteAppContainer(profile)
	defer windows.FreeSid(sid)

	base := t.TempDir()
	fwd := buildBinary(t, base, "forwarder2", fwd2Source)
	client := buildBinary(t, base, "fwdclient2", fwdClient2Source)

	// 宿主 echo（两连接并发）
	echoLn, err := net.Listen("tcp", "127.0.0.1:18761")
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

	// 两对管道（连接 0/1 各一对）
	const pipeBase = "GoalOS-Fwd2-2026"
	hostPipePair(t, pipeBase, 0, "127.0.0.1:18761")
	hostPipePair(t, pipeBase, 1, "127.0.0.1:18761")

	// AC 转发器
	fwdOut := base + `\fwd2.out`
	pi := spawnInAppContainer(t, sid, `"`+fwd+`" 127.0.0.1:18760 `+pipeBase, fwdOut)
	defer killAndWait(t, pi)
	time.Sleep(2 * time.Second)

	// 两客户端并发（payload 区分归属）
	type result struct {
		idx  int
		out  string
		code int
	}
	results := make(chan result, 2)
	for i, payload := range []string{"ping-ac-0", "ping-ac-1"} {
		go func(i int, p string) {
			code, out := runInAppContainer(t, sid, `"`+client+`" 127.0.0.1:18760 `+p, 30*time.Second)
			results <- result{i, out, code}
		}(i, payload)
	}
	okCount := 0
	for k := 0; k < 2; k++ {
		r := <-results
		want := fmt.Sprintf("FWD2-ECHO: ping-ac-%d", r.idx)
		if r.code == 0 && strings.Contains(r.out, want) {
			t.Logf("客户端 %d 并发全链=通（%q——归属正确）", r.idx, strings.TrimSpace(r.out))
			okCount++
		} else {
			t.Logf("客户端 %d 失败: exit=%d out=%q", r.idx, r.code, strings.TrimSpace(r.out))
		}
	}
	if okCount != 2 {
		t.Fatalf("并发全链未全通（%d/2）——fwd 侧=%q", okCount, strings.TrimSpace(readSpawnOut(fwdOut)))
	}
	t.Logf("②双单向管道并发转发=根治实证（2 客户端并发×双向，归属全对）")
}
