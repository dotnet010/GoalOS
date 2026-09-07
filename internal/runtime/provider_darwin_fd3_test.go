//go:build darwin

// provider_darwin_fd3_test.go——FD3 darwin Seatbelt 面契约测试（R-571 先红登记——
// R-1452 合法形态：本工作区无 mac 实机+无 darwin CI——测试=编译期验证+实机窗口待验；
// 断言规格钉死先行，实机补数=任何 mac 跑 `go test ./internal/runtime/ -run FD3` 即出数）。
// 断言=fd3-broker-设计.md §六 F1/F4 的 darwin 形态（unix socket 直连=linux 同构）：
//  D1：契约声明端点=Seatbelt 沙箱内经 unix socket 帧中继全链通（FD3_SOCK_PATH
//      收窄放行实证——SBPL 字面量形态）；
//  D2：未声明端点=OPEN_DENY；D3：沙箱内直接出站仍拒（deny network* 族在位）。
package runtime

import (
	"context"
	"net"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/goalos/goalos/internal/governance"
)

// darwinEchoServer 宿主 echo（broker 中继对拍目标）。
func darwinEchoServer(t *testing.T) string {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { ln.Close() })
	go func() {
		for {
			c, err := ln.Accept()
			if err != nil {
				return
			}
			go func(c net.Conn) {
				defer c.Close()
				buf := make([]byte, 32*1024)
				for {
					n, err := c.Read(buf)
					if err != nil {
						return
					}
					if _, err := c.Write(buf[:n]); err != nil {
						return
					}
				}
			}(c)
		}
	}()
	return ln.Addr().String()
}

// TestDarwin_FD3Relay D1+D3：声明端点=Seatbelt 内 unix socket 中继回显；出站仍拒。
func TestDarwin_FD3Relay(t *testing.T) {
	echoAddr := darwinEchoServer(t)
	ctx := context.Background()
	ws := t.TempDir()
	tmp := t.TempDir()
	p := NewDarwinSeatbeltProvider(ws, tmp)
	if err := p.Prepare(ctx, RuntimePlan{PlanID: "fd3-darwin", Tier: TierRestricted}); err != nil {
		t.Fatalf("Prepare: %v", err)
	}
	h, err := p.Acquire(ctx, LeaseRequest{GoalID: "fd3-darwin", ActionID: "r",
		Contract: &VerifiedContract{claims: governance.TokenClaims{
			GoalID: "fd3-darwin", ActionID: "r", Capabilities: []string{"shell.execute"},
			NetworkEndpoints: []string{echoAddr},
			IssuedAt:         time.Now().Unix(), ExpiresAt: time.Now().Add(time.Hour).Unix(),
		}}})
	if err != nil {
		t.Fatalf("Acquire: %v", err)
	}
	guard := NewHandleGuard(h)
	defer guard.Release(context.Background())
	if err := guard.Start(ctx); err != nil {
		t.Fatalf("Start（FD3 拉起面）: %v", err)
	}
	if err := guard.Precheck(ctx); err != nil {
		t.Fatalf("Precheck: %v", err)
	}
	mapData, err := os.ReadFile(filepath.Join(ws, "goalos-fd3-map.txt"))
	if err != nil {
		t.Fatalf("映射文件缺席: %v", err)
	}
	sock := strings.TrimPrefix(strings.Split(string(mapData), "\n")[0], "sock=")
	// D1：Seatbelt 内 fd3rt——探针载体=自身二进制（TARGET_BINARY 放行面=self；
	// __goalos-probe 拦截=probe_testmain_other_test.go）经 unix socket→broker→echo 回显
	self, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	res, err := guard.Execute(ctx, ExecuteRequest{ActionID: "fd3rt", ActionType: "process.exec",
		Params: map[string]string{"binary": self, "args": "__goalos-probe fd3rt " + sock + " " + echoAddr}})
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if !strings.Contains(res.Output, "ROUNDTRIP-OK") {
		t.Fatalf("D1：Seatbelt 内经 unix socket 中继回显失败——out=%q", res.Output)
	}
	// D3 对照：沙箱内直接出站仍拒（deny network* 族在位——FD3 变体不削弱）
	res, err = guard.Execute(ctx, ExecuteRequest{ActionID: "dial", ActionType: "process.exec",
		Params: map[string]string{"binary": self, "args": "__goalos-probe dial 192.0.2.1:80"}})
	if err != nil {
		t.Fatalf("Execute dial: %v", err)
	}
	if strings.Contains(res.Output, "PROBE-ERRNO=0") {
		t.Fatal("D3 CRITICAL：沙箱内直接出站竟通——FD3 变体削弱了网络禁闭")
	}
}
