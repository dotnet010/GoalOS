// broker.go——FD3 daemon 侧治理中继（S2——设计=开发计划/fd3-broker-设计.md §三/§四）。
//
// 治理语义（fail-closed 双层不冒充）：
//   - 端点白名单=契约声明集——未声明=OPEN_DENY 拒绝帧+审计留痕回调（onDeny）；
//   - 拨号=注入 dial 函数（生产=zone dialer——网域分类/留痕同源，R-1643/R-1650 v4；
//     nil=net.DialTimeout 直连兜底——仅测试/先行面，生产接线必须注入）；
//   - 协议纪律：首帧非 OPEN=协议违规=断开（不猜不放）；
//   - OS 层无网+broker 拒绝=双闸——agent 绕过 broker 直连=零 capability 全拒（R-1648）。
package fd3

import (
	"context"
	"fmt"
	"io"
	"net"
	"sync"
	"time"
)

// DialFunc 拨号函数签名（zone dialer 注入面——生产=ZoneDialer.DialContext 适配）。
type DialFunc func(ctx context.Context, network, addr string) (net.Conn, error)

// frameConn 帧面连接抽象（协议核心的传输中立缝——R-1695-2「FD3 测试分层纳管」）：
// 协议与帧解包测试=通用单测车道（零 OS 依赖，内存管道直驱本接口，随常规 PR/CI
// 三平台全量触发）；物理 socket/路径边界测试=平台专项特测车道（build tag
// platformtest）。两平台 *Conn 形态（unix=单句柄 / windows=双单向句柄对）均隐式
// 满足本接口——接口只收敛 broker 协议的承载面，不掩盖平台传输差异。
type frameConn interface {
	ReadFrame() (Frame, error)
	WriteFrame(Frame) error
	Close() error
}

// Broker daemon 侧 broker（治理中继——契约端点校验+注入拨号+审计留痕）。
type Broker struct {
	allowed map[string]bool // 契约声明端点集（"host:port" 精确匹配）
	dial    DialFunc
	onDeny  func(endpoint, reason string) // 审计留痕回调（nil=仅拒绝不留痕——调用方纪律）
}

// NewBroker 构造。allowed=契约声明端点集；dial=nil 时=net.DialTimeout 直连兜底
//（测试/先行面——生产接线注入 zone dialer，见 main.go 接线点）；onDeny=审计回调。
func NewBroker(allowed []string, dial DialFunc, onDeny func(endpoint, reason string)) *Broker {
	m := make(map[string]bool, len(allowed))
	for _, ep := range allowed {
		m[ep] = true
	}
	if dial == nil {
		dial = func(ctx context.Context, network, addr string) (net.Conn, error) {
			return (&net.Dialer{Timeout: 5 * time.Second}).DialContext(ctx, network, addr)
		}
	}
	return &Broker{allowed: m, dial: dial, onDeny: onDeny}
}

// Serve Accept 循环（每连接一个 handle goroutine——R-1662 v2 并发形态）。
func (b *Broker) Serve(ln *Listener) {
	for {
		conn, err := ln.Accept()
		if err != nil {
			return // 监听器关闭=退出
		}
		go b.handle(conn)
	}
}

// handle 单连接全生命周期：OPEN 校验→双向泵→收尾。
// 形参=frameConn（非 *Conn）——协议核心与物理传输解耦的缝：生产路径同为
// broker.Serve 传入的平台 Conn（隐式满足），测试路径经内存管道直驱（零 OS 依赖）。
func (b *Broker) handle(c frameConn) {
	defer c.Close()
	// 首帧必须 OPEN（协议纪律——fail-closed）
	f, err := c.ReadFrame()
	if err != nil || f.Op != OpOpen {
		return // 断开=违规处置（不猜不放）
	}
	endpoint := string(f.Payload)
	if !b.allowed[endpoint] {
		if b.onDeny != nil {
			b.onDeny(endpoint, "契约未声明端点（network_endpoints 白名单外）")
		}
		_ = c.WriteFrame(Frame{Op: OpOpenDeny, Payload: []byte("契约未声明端点")})
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	target, err := b.dial(ctx, "tcp", endpoint)
	cancel()
	if err != nil {
		_ = c.WriteFrame(Frame{Op: OpOpenDeny, Payload: []byte(fmt.Sprintf("目标不可达: %v", err))})
		return
	}
	defer target.Close()
	if err := c.WriteFrame(Frame{Op: OpOpenOK}); err != nil {
		return
	}

	// 双向泵：客户端帧→目标字节流 / 目标字节流→客户端帧
	var wg sync.WaitGroup
	wg.Add(2)
	// 客户端→目标（DATA 载荷写入；CLOSE=半关；流断=收尾）
	go func() {
		defer wg.Done()
	pump:
		for {
			f, err := c.ReadFrame()
			if err != nil {
				break
			}
			switch f.Op {
			case OpData:
				if _, err := target.Write(f.Payload); err != nil {
					break pump // 目标已死=泵终止（SA4011 修正——break 出 switch 不终止循环）
				}
			case OpClose:
				if tc, ok := target.(*net.TCPConn); ok {
					_ = tc.CloseWrite() // 半关语义（读侧继续——HTTP/1.1 族需要）
				}
				break pump
			default:
				// 未知 op=忽略（前向兼容——不加码）
			}
		}
	}()
	// 目标→客户端（字节流成帧——32KiB 块）
	go func() {
		defer wg.Done()
		buf := make([]byte, 32*1024)
		for {
			n, err := target.Read(buf)
			if n > 0 {
				if werr := c.WriteFrame(Frame{Op: OpData, Payload: buf[:n]}); werr != nil {
					return
				}
			}
			if err != nil {
				if err == io.EOF {
					_ = c.WriteFrame(Frame{Op: OpClose})
				}
				return
			}
		}
	}()
	wg.Wait()
}
