// fd3_broker_test.go——FD3 broker 治理面契约测试（S2——R-571 先红）。
// 断言来源=开发计划/fd3-broker-设计.md §四治理面+F2/F1 矩阵：
//  F2：未声明端点=OPEN_DENY 拒绝帧+审计留痕回调（治理 fail-closed——不静默不透传）；
//  F1-lite：声明端点=OPEN_OK+DATA 双向中继（回环 echo 服务对拍）；
//  协议纪律：OPEN 前的 DATA=协议违规=连接断开（fail-closed 不猜）。
//
// 车道（R-1695 ②「FD3 测试分层纳管——协议与物理传输解耦」）：**通用单测车道**——
// 本文件零 OS 依赖（无 build tag）：不 bind socket、不落文件系统路径、不触 sun_path
// 预算与目录权限面，经内存管道（net.Pipe）直驱 broker 协议核心（frameConn 缝），
// 随常规 PR/CI 三平台（linux/darwin/windows）全量触发。
// 物理传输面（socket bind/路径形态/目录权限/并发连接隔离）=平台专项特测车道
// （build tag `platformtest`——fd3_contract_test.go / transport_dir_darwin_test.go）。
package fd3

import (
	"context"
	"fmt"
	"net"
	"strings"
	"testing"
	"time"
)

// pipeConn 内存帧管道端点（通用车道夹具——frameConn 的零 OS 实现）。
// 生产对偶=平台 *Conn（unix 单句柄 / windows 双单向句柄对）——协议核心对两者同一。
type pipeConn struct{ rw net.Conn }

func (p *pipeConn) ReadFrame() (Frame, error) { return ReadFrame(p.rw) }
func (p *pipeConn) WriteFrame(f Frame) error  { return WriteFrame(p.rw, f) }
func (p *pipeConn) Close() error              { return p.rw.Close() }

// memPair 内存帧管道对（client=测试侧，server=broker 侧）。
func memPair() (client, server *pipeConn) {
	c, s := net.Pipe()
	return &pipeConn{rw: c}, &pipeConn{rw: s}
}

// memEchoEndpoint 内存回环目标的白名单键（非网络地址——dial 被注入，字符串仅作
// 契约端点集匹配键；沿用 test-net-1 之外的可读形态，避免误读为真实拨号面）。
const memEchoEndpoint = "mem-echo://fd3-test"

// memEcho 内存回环 echo 目标：注入 dial 每次返回一条新管道的对端，服务端跑字节流回显
//（broker 双向泵的对拍面——零 OS 依赖，取代真实 TCP 回环监听的通用车道形态）。
func memEcho(t *testing.T) DialFunc {
	t.Helper()
	return func(_ context.Context, _, _ string) (net.Conn, error) {
		srv, cli := net.Pipe()
		go func() {
			defer srv.Close()
			buf := make([]byte, 64*1024)
			for {
				n, err := srv.Read(buf)
				if n > 0 {
					if _, werr := srv.Write(buf[:n]); werr != nil {
						return
					}
				}
				if err != nil {
					return
				}
			}
		}()
		return cli, nil
	}
}

// dialThrough 测试侧 OPEN+读回应（内存管道直驱——不经 Dial/Listen）。
func dialThrough(t *testing.T, client *pipeConn, endpoint string) (Frame, *pipeConn) {
	t.Helper()
	if err := client.WriteFrame(Frame{Op: OpOpen, Payload: []byte(endpoint)}); err != nil {
		t.Fatalf("OPEN 写: %v", err)
	}
	resp, err := client.ReadFrame()
	if err != nil {
		t.Fatalf("读 OPEN 响应: %v", err)
	}
	return resp, client
}

// TestFD3Broker_DenyUndeclared F2：未声明端点=OPEN_DENY+审计留痕。
func TestFD3Broker_DenyUndeclared(t *testing.T) {
	var denies []string
	broker := NewBroker([]string{memEchoEndpoint}, memEcho(t), func(ep, reason string) {
		denies = append(denies, ep+"|"+reason)
	})
	client, server := memPair()
	go broker.handle(server)

	resp, conn := dialThrough(t, client, "192.0.2.1:80") // 未声明
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
	broker := NewBroker([]string{memEchoEndpoint}, memEcho(t), nil)
	client, server := memPair()
	go broker.handle(server)

	resp, conn := dialThrough(t, client, memEchoEndpoint)
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
	broker := NewBroker([]string{memEchoEndpoint}, memEcho(t), nil)
	client, server := memPair()
	go broker.handle(server)

	defer client.Close()
	// 直接 DATA（未 OPEN）——应被断开（读失败=fail-closed）
	if err := client.WriteFrame(Frame{Op: OpData, Payload: []byte("premature")}); err != nil {
		t.Fatalf("DATA 写: %v", err)
	}
	if _, err := client.ReadFrame(); err == nil {
		t.Fatal("OPEN 前 DATA 未被断开——协议纪律失守")
	}
}

// TestFD3Broker_ZoneDialerInjection zone dialer 注入面：broker 拨号必须走注入的
// dial 函数（网域分类/留痕同源不旁路——R-1643/R-1650 v4 接线纪律）。
func TestFD3Broker_ZoneDialerInjection(t *testing.T) {
	base := memEcho(t)
	dialed := make(chan string, 1)
	injectDial := func(ctx context.Context, network, addr string) (net.Conn, error) {
		dialed <- addr
		return base(ctx, network, addr)
	}
	broker := NewBroker([]string{memEchoEndpoint}, injectDial, nil)
	client, server := memPair()
	go broker.handle(server)

	resp, conn := dialThrough(t, client, memEchoEndpoint)
	defer conn.Close()
	if resp.Op != OpOpenOK {
		t.Fatalf("OPEN 应 OK: %q", resp.Payload)
	}
	select {
	case got := <-dialed:
		if got != memEchoEndpoint {
			t.Fatalf("注入 dial 收到的目标=%q（应=%q）", got, memEchoEndpoint)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("注入 dial 未被调用——拨号旁路了 zone dialer 注入面")
	}
	fmt.Println("注入面实证 ok")
}

// TestFD3Broker_DialFailureDeny 拨号失败=OPEN_DENY（fail-closed 不静默放行：
// 端点已声明但目标不可达——拒绝帧携带理由，治理面不留半开连接）。
// 注：拨号失败不走 onDeny 回调（设计 §三——留痕责任在 zone dialer 一侧；
// onDeny=契约声明面未声明端点的审计面）。
func TestFD3Broker_DialFailureDeny(t *testing.T) {
	failingDial := func(_ context.Context, _, _ string) (net.Conn, error) {
		return nil, fmt.Errorf("目标不可达（注入故障——通用车道零 OS 依赖形态）")
	}
	broker := NewBroker([]string{memEchoEndpoint}, failingDial, nil)
	client, server := memPair()
	go broker.handle(server)

	resp, conn := dialThrough(t, client, memEchoEndpoint)
	defer conn.Close()
	if resp.Op != OpOpenDeny {
		t.Fatalf("拨号失败应 OPEN_DENY，实得 op=0x%02X", resp.Op)
	}
	if !strings.Contains(string(resp.Payload), "目标不可达") {
		t.Errorf("拒绝帧应携带理由（治理留痕同源），实得 payload=%q", resp.Payload)
	}
}
