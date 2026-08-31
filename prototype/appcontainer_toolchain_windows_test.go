//go:build prototype && windows

// appcontainer_toolchain_windows_test.go——顾问疑虑①（DLL/依赖加载断层）+
// 疑虑②（Loopback 硬阻断）实机裁决（2026-08-31，PM 指令：真机实测）。
//
// 疑虑①裁决设计：真实解释器双靶——python（用户目录驻留，依赖同目录
// python3xx.dll+Lib 树=运行期读面）与 node（Program Files 驻留），
// 各测「未授予/授予」两态——把「exe 能启动」与「工具链能跑通业务」分开钉。
//
// 疑虑②裁决设计：loopback 探针四场景——同进程自拨/同 AC 跨进程互连/
// 宿主听 AC 拨/AC 听宿主拨——全矩阵出数，不预设结论。
package prototype

import (
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"golang.org/x/sys/windows"
)

// loopbackSource 回环探针（三模式：selfdial 同进程自拨 / listen 监听一次 /
// dial 主动连接——跨进程场景由测试编排）。
const loopbackSource = `package main

import (
	"fmt"
	"net"
	"os"
	"time"
)

func main() {
	switch os.Args[1] {
	case "selfdial":
		ln, err := net.Listen("tcp", "127.0.0.1:0")
		if err != nil {
			fmt.Println("SELF-LISTEN-FAIL: " + err.Error())
			os.Exit(2)
		}
		defer ln.Close()
		go func() {
			c, err := ln.Accept()
			if err != nil {
				return
			}
			c.Write([]byte("x"))
			c.Close()
		}()
		conn, err := net.DialTimeout("tcp", ln.Addr().String(), 3*time.Second)
		if err != nil {
			fmt.Println("SELF-DIAL-FAIL: " + err.Error())
			os.Exit(3)
		}
		buf := make([]byte, 1)
		conn.Read(buf)
		fmt.Println("LOOPBACK-SELF-OK")
	case "listen":
		ln, err := net.Listen("tcp", os.Args[2])
		if err != nil {
			fmt.Println("LISTEN-FAIL: " + err.Error())
			os.Exit(2)
		}
		fmt.Println("LISTENING")
		ln.(*net.TCPListener).SetDeadline(time.Now().Add(20 * time.Second))
		c, err := ln.Accept()
		if err != nil {
			fmt.Println("ACCEPT-TIMEOUT: " + err.Error())
			os.Exit(3)
		}
		fmt.Println("ACCEPT-OK")
		c.Close()
	case "dial":
		conn, err := net.DialTimeout("tcp", os.Args[2], 5*time.Second)
		if err != nil {
			fmt.Println("DIAL-FAIL: " + err.Error())
			os.Exit(3)
		}
		fmt.Println("DIAL-OK")
		conn.Close()
	}
}
`

// sleeperSource 长活进程（profile 生命周期测试用）。
const sleeperSource = `package main

import (
	"fmt"
	"time"
)

func main() {
	fmt.Println("SLEEPER-UP")
	time.Sleep(60 * time.Second)
}
`

// TestAppContainer_DependencyLoad 疑虑①裁决——真实解释器依赖加载面。
func TestAppContainer_DependencyLoad(t *testing.T) {
	const profile = "GoalOS-Spike-AC-Dep"
	deleteAppContainer(profile)
	sid := createAppContainer(t, profile)
	defer deleteAppContainer(profile)
	defer windows.FreeSid(sid)
	sidStr := sidString(t, sid)

	pythonExe, pyErr := exec.LookPath("python")
	nodeExe, ndErr := exec.LookPath("node")
	if pyErr != nil {
		t.Skip("python 不在 PATH——跳过 python 靶")
	}
	t.Logf("靶标: python=%s node=%v", pythonExe, nodeExe)
	pyRoot := filepath.Dir(pythonExe)

	// —— A1. python 未授予：loader 面裁决（预期=启动即死，记录精确失败签名）——
	code, out := runInAppContainer(t, sid, `"`+pythonExe+`" -c "print('py-ac-ok')"`, 60*time.Second)
	if code == 0 && strings.Contains(out, "py-ac-ok") {
		t.Logf("A1 CRITICAL-反预期：python 未授予即跑通——用户目录读面比测绘所示更宽: %q", strings.TrimSpace(out))
	} else {
		t.Logf("A1 python 未授予=失败（exit=%d 0x%08X）out=%q", code, uint32(code), strings.TrimSpace(out))
	}

	// —— A2. 授予安装根 RX：工具链完整跑通 ——
	icaclsGrant(t, sidStr, pyRoot, "(OI)(CI)(RX)", false)
	defer icaclsGrant(t, sidStr, pyRoot, "", true)
	code, out = runInAppContainer(t, sid, `"`+pythonExe+`" -c "import sys; print('py-ac-ok', sys.version_info[0], sys.version_info[1])"`, 60*time.Second)
	if code != 0 || !strings.Contains(out, "py-ac-ok") {
		t.Fatalf("A2：授予后 python 仍失败——「最小运行时授权集」解法存疑: exit=%d %q", code, out)
	}
	t.Logf("A2 python 授予后=跑通（%q）", strings.TrimSpace(out))

	// —— B1. node 未授予（Program Files 驻留——系统目录读面是否覆盖 Program Files）——
	if ndErr == nil {
		code, out = runInAppContainer(t, sid, `"`+nodeExe+`" -e "console.log('node-ac-ok')"`, 60*time.Second)
		if code == 0 && strings.Contains(out, "node-ac-ok") {
			t.Logf("B1 node 未授予=跑通（Program Files 在默认读面内）")
		} else {
			t.Logf("B1 node 未授予=失败（exit=%d 0x%08X）out=%q——Program Files 不在默认读面", code, uint32(code), strings.TrimSpace(out))
		}
	}
}

// TestAppContainer_Loopback 疑虑②裁决——回环四场景矩阵。
func TestAppContainer_Loopback(t *testing.T) {
	const profile = "GoalOS-Spike-AC-Loop"
	deleteAppContainer(profile)
	sid := createAppContainer(t, profile)
	defer deleteAppContainer(profile)
	defer windows.FreeSid(sid)

	base := t.TempDir()
	probe := buildBinary(t, base, "loopback", loopbackSource)

	// —— S1. 同进程自拨（listen+dial 同一 AC 进程内）——
	code, out := runInAppContainer(t, sid, `"`+probe+`" selfdial`, 30*time.Second)
	s1 := "拒"
	if strings.Contains(out, "LOOPBACK-SELF-OK") {
		s1 = "通"
	}
	t.Logf("S1 同进程自拨 127.0.0.1 = %s（exit=%d out=%q）", s1, code, strings.TrimSpace(out))

	// —— S2. 同 AC 跨进程互连（listener + dialer 同 profile）——
	const addr = "127.0.0.1:18731"
	listenOut := filepath.Join(base, "listener.out")
	pi := spawnInAppContainer(t, sid, `"`+probe+`" listen `+addr, listenOut)
	defer killAndWait(t, pi)
	time.Sleep(2 * time.Second) // 等监听就绪
	code, out = runInAppContainer(t, sid, `"`+probe+`" dial `+addr, 30*time.Second)
	s2 := "拒"
	if strings.Contains(out, "DIAL-OK") {
		s2 = "通"
	}
	t.Logf("S2 同 AC 跨进程互连 = %s（dialer exit=%d out=%q）", s2, code, strings.TrimSpace(out))
	killAndWait(t, pi)
	if data, _ := os.ReadFile(listenOut); true {
		t.Logf("S2 listener 侧输出=%q", strings.TrimSpace(string(data)))
	}

	// —— S3. 宿主听 + AC 拨 ——
	hostLn, err := net.Listen("tcp", "127.0.0.1:18732")
	if err != nil {
		t.Fatalf("宿主监听失败: %v", err)
	}
	defer hostLn.Close()
	go func() { c, _ := hostLn.Accept(); if c != nil { c.Close() } }()
	code, out = runInAppContainer(t, sid, `"`+probe+`" dial 127.0.0.1:18732`, 30*time.Second)
	s3 := "拒"
	if strings.Contains(out, "DIAL-OK") {
		s3 = "通"
	}
	t.Logf("S3 宿主听+AC 拨 = %s（exit=%d out=%q）", s3, code, strings.TrimSpace(out))

	// —— S4. AC 听 + 宿主拨 ——
	const addr4 = "127.0.0.1:18733"
	listenOut4 := filepath.Join(base, "listener4.out")
	pi4 := spawnInAppContainer(t, sid, `"`+probe+`" listen `+addr4, listenOut4)
	defer killAndWait(t, pi4)
	time.Sleep(2 * time.Second)
	conn4, dialErr := net.DialTimeout("tcp", addr4, 5*time.Second)
	s4 := "拒"
	if dialErr == nil {
		s4 = "通"
		conn4.Close()
	}
	t.Logf("S4 AC 听+宿主拨 = %s（dialErr=%v）", s4, dialErr)
	killAndWait(t, pi4)
	if data, _ := os.ReadFile(listenOut4); true {
		t.Logf("S4 listener 侧输出=%q", strings.TrimSpace(string(data)))
	}
}
