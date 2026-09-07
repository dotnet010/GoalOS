//go:build windows

// provider_winac_fd3_test.go——FD3 沙箱内中继全链契约测试（S3——R-571）。
// 断言来源=开发计划/fd3-broker-设计.md §六 F1/F4/F5：
//  F1：契约声明端点=AC 内 roundtrip 全链通（探针 dial+写+读回显——字节级中继实证）；
//  F4：未声明端点/未拉中继=连接即拒（fail-closed——无静默挂起）；
//  F5：边界不削弱对照——直接出站仍拒（WSAEACCES）与 F1 同 session 并存。
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

// winACContractNet 带端点声明的契约构造。
func winACContractNet(goalID, actionID string, caps []string, endpoints []string) *VerifiedContract {
	return &VerifiedContract{claims: governance.TokenClaims{
		GoalID: goalID, ActionID: actionID, Capabilities: caps,
		NetworkEndpoints: endpoints,
		IssuedAt:         time.Now().Unix(), ExpiresAt: time.Now().Add(time.Hour).Unix(),
	}}
}

// fd3EchoServer 宿主回环 echo（FD3 中继对拍目标）。
func fd3EchoServer(t *testing.T) string {
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

// TestWinACProvider_FD3Relay F1+F5：声明端点=AC 内字节级中继通；同 session 直接出站仍拒。
func TestWinACProvider_FD3Relay(t *testing.T) {
	echoAddr := fd3EchoServer(t) // 宿主 127.0.0.1:<port>
	ctx := context.Background()
	ws := t.TempDir()
	p := NewWinACProvider(ws, t.TempDir(), nil)
	if err := p.Prepare(ctx, RuntimePlan{PlanID: "fd3-relay", Tier: TierRestricted}); err != nil {
		t.Fatalf("Prepare: %v", err)
	}
	h, err := p.Acquire(ctx, LeaseRequest{GoalID: "fd3-relay", ActionID: "r",
		Contract: winACContractNet("fd3-relay", "r", []string{"shell.execute"}, []string{echoAddr})})
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
	// 读实际映射（回环同命名空间——声明端口可能被宿主占用，实际端口=映射文件权威）
	mapData, err := os.ReadFile(filepath.Join(ws, "goalos-fd3-map.txt"))
	if err != nil {
		t.Fatalf("映射文件缺席: %v", err)
	}
	line := strings.TrimSpace(strings.Split(string(mapData), "\n")[0])
	lport := line[:strings.Index(line, "=")]
	// F1：AC 内 roundtrip——经 fd3d→管道→broker→宿主 echo 全链字节级回显
	res := execProbe(t, guard, "__goalos-probe roundtrip 127.0.0.1:"+lport)
	if !strings.Contains(res.Output, "ROUNDTRIP-OK") {
		t.Fatalf("F1：AC 内中继回显失败——out=%q errno=%d", res.Output, errnoOf(t, res.Output))
	}
	// F5 对照：同 session 直接出站仍拒（中继面不削弱边界）
	res = execProbe(t, guard, "__goalos-probe dial 192.0.2.1:80")
	if errnoOf(t, res.Output) == 0 {
		t.Fatal("F5 CRITICAL：直接出站竟通——中继面削弱了零 capability 边界")
	}
}

// TestWinACProvider_FD3Undeclared F4：未声明端点=无 fd3d=回环端口连接即拒。
func TestWinACProvider_FD3Undeclared(t *testing.T) {
	echoAddr := fd3EchoServer(t)
	port := echoAddr[strings.LastIndex(echoAddr, ":")+1:]
	ctx := context.Background()
	p := NewWinACProvider(t.TempDir(), t.TempDir(), nil)
	if err := p.Prepare(ctx, RuntimePlan{PlanID: "fd3-deny", Tier: TierRestricted}); err != nil {
		t.Fatalf("Prepare: %v", err)
	}
	// 契约无端点声明
	guard := winACReady(t, p, "fd3-deny", "d", []string{"shell.execute"})
	defer guard.Release(context.Background())
	res := execProbe(t, guard, "__goalos-probe roundtrip 127.0.0.1:"+port)
	if errnoOf(t, res.Output) == 0 || strings.Contains(res.Output, "ROUNDTRIP-OK") {
		t.Fatalf("F4 CRITICAL：未声明端点竟中继成功——治理面失守: %q", res.Output)
	}
}
