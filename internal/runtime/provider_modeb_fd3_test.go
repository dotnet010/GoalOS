//go:build linux

// provider_modeb_fd3_test.go——FD3 Linux 模式 B 全链契约测试（S4——R-571）。
// 断言=fd3-broker-设计.md §六 F1/F4 的 Linux 形态：
//  U1：契约声明端点=模式 B 沙箱内经 unix socket FD3 帧中继全链通（fd3rt 探针
//      字节级回显——真实沙箱面，不经 mock）；
//  U2：未声明端点=OPEN_DENY（broker 治理 fail-closed——PROBE-ERRNO=-5）；
//  U3：边界不削弱对照——沙箱内直接出站仍拒（EACCES=13）。
// 形态差异诚实标注：Linux=沙箱进程对 unix socket 直说 FD3 帧（无 fd3d 转发器——
// 模式 B seccomp AF_UNIX 白名单放行直连；无回环透明）。
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

// modeBEchoServer 宿主 echo（broker 中继对拍目标）。
func modeBEchoServer(t *testing.T) string {
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

// modeBContractNet 带端点声明的契约构造。
func modeBContractNet(goalID, actionID string, endpoints []string) *VerifiedContract {
	return &VerifiedContract{claims: governance.TokenClaims{
		GoalID: goalID, ActionID: actionID, Capabilities: []string{"shell.execute"},
		NetworkEndpoints: endpoints,
		IssuedAt:         time.Now().Unix(), ExpiresAt: time.Now().Add(time.Hour).Unix(),
	}}
}

// TestModeB_FD3Relay L1+U3：声明端点=沙箱内 unix socket 帧中继通；出站仍拒。
func TestModeB_FD3Relay(t *testing.T) {
	if !modeBAvailable() {
		t.Skip("模式 B 不可用（landlock ABI 缺席/非 amd64——R-1452 合法先红形态）")
	}
	echoAddr := modeBEchoServer(t)
	ctx := context.Background()
	ws := t.TempDir()
	tmp := t.TempDir()
	p := NewModeBProvider(ws, tmp)
	if err := p.Prepare(ctx, RuntimePlan{PlanID: "fd3-linux", Tier: TierRestricted}); err != nil {
		t.Fatalf("Prepare: %v", err)
	}
	h, err := p.Acquire(ctx, LeaseRequest{GoalID: "fd3-linux", ActionID: "r",
		Contract: modeBContractNet("fd3-linux", "r", []string{echoAddr})})
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
	// 读映射文件取 socket 路径
	mapData, err := os.ReadFile(filepath.Join(ws, "goalos-fd3-map.txt"))
	if err != nil {
		t.Fatalf("映射文件缺席: %v", err)
	}
	sock := strings.TrimPrefix(strings.Split(string(mapData), "\n")[0], "sock=")
	if !strings.HasPrefix(sock, tmp) {
		t.Fatalf("socket 路径不在 tmpDir 授予面: %q", sock)
	}
	// U1：沙箱内 fd3rt——unix socket→broker→宿主 echo 字节级回显
	self, _ := os.Executable()
	res, err := guard.Execute(ctx, ExecuteRequest{ActionID: "fd3rt", ActionType: "process.exec",
		Params: map[string]string{"binary": self, "args": "__goalos-probe fd3rt " + sock + " " + echoAddr}})
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if !strings.Contains(res.Output, "ROUNDTRIP-OK") {
		t.Fatalf("U1：沙箱内经 unix socket 中继回显失败——out=%q", res.Output)
	}
	// L3 对照：同 session 沙箱内直接出站仍拒（EACCES=13——seccomp 域名白名单在位）
	res, err = guard.Execute(ctx, ExecuteRequest{ActionID: "dial", ActionType: "process.exec",
		Params: map[string]string{"binary": self, "args": "__goalos-probe dial 192.0.2.1:80"}})
	if err != nil {
		t.Fatalf("Execute dial: %v", err)
	}
	if strings.Contains(res.Output, "PROBE-ERRNO=0") {
		t.Fatal("L3 CRITICAL：沙箱内直接出站竟通——中继面削弱了 seccomp 边界")
	}
}

// TestModeB_FD3Undeclared U2：未声明端点=broker OPEN_DENY（PROBE-ERRNO=-5）。
func TestModeB_FD3Undeclared(t *testing.T) {
	if !modeBAvailable() {
		t.Skip("模式 B 不可用——R-1452 合法先红形态")
	}
	echoAddr := modeBEchoServer(t)
	ctx := context.Background()
	ws := t.TempDir()
	tmp := t.TempDir()
	p := NewModeBProvider(ws, tmp)
	if err := p.Prepare(ctx, RuntimePlan{PlanID: "fd3-deny", Tier: TierRestricted}); err != nil {
		t.Fatalf("Prepare: %v", err)
	}
	// 契约声明端点=echoAddr，但探针 OPEN 一个未声明端点
	h, err := p.Acquire(ctx, LeaseRequest{GoalID: "fd3-deny", ActionID: "d",
		Contract: modeBContractNet("fd3-deny", "d", []string{echoAddr})})
	if err != nil {
		t.Fatalf("Acquire: %v", err)
	}
	guard := NewHandleGuard(h)
	defer guard.Release(context.Background())
	if err := guard.Start(ctx); err != nil {
		t.Fatalf("Start: %v", err)
	}
	if err := guard.Precheck(ctx); err != nil {
		t.Fatalf("Precheck: %v", err)
	}
	mapData, _ := os.ReadFile(filepath.Join(ws, "goalos-fd3-map.txt"))
	sock := strings.TrimPrefix(strings.Split(string(mapData), "\n")[0], "sock=")
	self, _ := os.Executable()
	res, err := guard.Execute(ctx, ExecuteRequest{ActionID: "fd3rt", ActionType: "process.exec",
		Params: map[string]string{"binary": self, "args": "__goalos-probe fd3rt " + sock + " 127.0.0.1:1"}}) // 未声明
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if !strings.Contains(res.Output, "PROBE-ERRNO=-5") {
		t.Fatalf("U2：未声明端点应 broker 拒绝（-5），实得 out=%q", res.Output)
	}
}
