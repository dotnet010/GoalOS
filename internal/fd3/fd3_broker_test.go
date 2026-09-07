//go:build windows || linux || darwin

// fd3_broker_test.go——FD3 broker 治理面契约测试（S2——R-571 先红）。
// 断言来源=开发计划/fd3-broker-设计.md §四治理面+F2/F1 矩阵：
//  F2：未声明端点=OPEN_DENY 拒绝帧+审计留痕回调（治理 fail-closed——不静默不透传）；
//  F1-lite：声明端点=OPEN_OK+DATA 双向中继（真实回环 echo 服务对拍）；
//  协议纪律：OPEN 前的 DATA=协议违规=连接断开（fail-closed 不猜）。
package fd3

import (
	"context"
	"fmt"
	"net"
	"strings"
	"testing"
	"time"
)

// echoServer 测试回环 echo 服务（真实 TCP 目标——broker 中继对拍面）。
func echoServer(t *testing.T) string {
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
				buf := make([]byte, 64*1024)
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

// dialThrough 客户端 OPEN+读回应。
func dialThrough(t *testing.T, base, endpoint string) (Frame, *Conn) {
	t.Helper()
	conn, err := Dial(base)
	if err != nil {
		t.Fatalf("Dial: %v", err)
	}
	if err := conn.WriteFrame(Frame{Op: OpOpen, Payload: []byte(endpoint)}); err != nil {
		t.Fatalf("OPEN 写: %v", err)
	}
	resp, err := conn.ReadFrame()
	if err != nil {
		t.Fatalf("读 OPEN 响应: %v", err)
	}
	return resp, conn
}

// TestFD3Broker_DenyUndeclared F2：未声明端点=OPEN_DENY+审计留痕。
func TestFD3Broker_DenyUndeclared(t *testing.T) {
	echoAddr := echoServer(t)
	var denies []string
	broker := NewBroker([]string{echoAddr}, nil, func(ep, reason string) {
		denies = append(denies, ep+"|"+reason)
	})
	ln, err := Listen(t.TempDir(), "FD3-Broker-F2")
	if err != nil {
		t.Fatal(err)
	}
	defer ln.Close()
	go broker.Serve(ln)

	resp, conn := dialThrough(t, ln.Name(), "192.0.2.1:80") // 未声明
	defer conn.Close()
	if resp.Op != OpOpenDeny {
		t.Fatalf("未声明端点应 OPEN_DENY，实得 op=0x%02X", resp.Op)
	}
	if len(denies) == 0 || !strings.Contains(denies[0], "192.0.2.1:80") {
		t.Fatalf("审计留痕缺失/错位: %v", denies)
	}
}

// TestFD3Broker_RelayDeclared F1-lite：声明端点全链中继（broker→echo 对拍）。
func TestFD3Broker_RelayDeclared(t *testing.T) {
	echoAddr := echoServer(t)
	broker := NewBroker([]string{echoAddr}, nil, nil)
	ln, err := Listen(t.TempDir(), "FD3-Broker-F1")
	if err != nil {
		t.Fatal(err)
	}
	defer ln.Close()
	go broker.Serve(ln)

	resp, conn := dialThrough(t, ln.Name(), echoAddr)
	defer conn.Close()
	if resp.Op != OpOpenOK {
		t.Fatalf("声明端点应 OPEN_OK，实得 op=0x%02X payload=%q", resp.Op, resp.Payload)
	}
	// DATA 双向：写 "hello-fd3" 应原样回（经 broker→echo→broker→客户端）
	payload := []byte("hello-fd3")
	if err := conn.WriteFrame(Frame{Op: OpData, Payload: payload}); err != nil {
		t.Fatalf("DATA 写: %v", err)
	}
	resp2, err := conn.ReadFrame()
	if err != nil {
		t.Fatalf("DATA 读: %v", err)
	}
	if resp2.Op != OpData || string(resp2.Payload) != string(payload) {
		t.Fatalf("中继失真: op=0x%02X payload=%q", resp2.Op, resp2.Payload)
	}
}

// TestFD3Broker_ProtocolDiscipline 协议纪律：OPEN 前 DATA=违规=连接断开。
func TestFD3Broker_ProtocolDiscipline(t *testing.T) {
	echoAddr := echoServer(t)
	broker := NewBroker([]string{echoAddr}, nil, nil)
	ln, err := Listen(t.TempDir(), "FD3-Broker-F3d")
	if err != nil {
		t.Fatal(err)
	}
	defer ln.Close()
	go broker.Serve(ln)

	conn, err := Dial(ln.Name())
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	// 直接 DATA（未 OPEN）——应被断开（读失败=fail-closed）
	if err := conn.WriteFrame(Frame{Op: OpData, Payload: []byte("premature")}); err != nil {
		t.Fatalf("DATA 写: %v", err)
	}
	if _, err := conn.ReadFrame(); err == nil {
		t.Fatal("OPEN 前 DATA 未被断开——协议纪律失守")
	}
}

// TestFD3Broker_ZoneDialerInjection zone dialer 注入面：broker 拨号必须走注入的
// dial 函数（网域分类/留痕同源不旁路——R-1643/R-1650 v4 接线纪律）。
func TestFD3Broker_ZoneDialerInjection(t *testing.T) {
	echoAddr := echoServer(t)
	dialed := make(chan string, 1)
	injectDial := func(_ context.Context, _, addr string) (net.Conn, error) {
		dialed <- addr
		return net.DialTimeout("tcp", addr, 3*time.Second)
	}
	broker := NewBroker([]string{echoAddr}, injectDial, nil)
	ln, err := Listen(t.TempDir(), "FD3-Broker-Inj")
	if err != nil {
		t.Fatal(err)
	}
	defer ln.Close()
	go broker.Serve(ln)

	resp, conn := dialThrough(t, ln.Name(), echoAddr)
	defer conn.Close()
	if resp.Op != OpOpenOK {
		t.Fatalf("OPEN 应 OK: %q", resp.Payload)
	}
	select {
	case got := <-dialed:
		if got != echoAddr {
			t.Fatalf("注入 dial 收到的目标=%q（应=%q）", got, echoAddr)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("注入 dial 未被调用——拨号旁路了 zone dialer 注入面")
	}
	fmt.Println("注入面实证 ok")
}
