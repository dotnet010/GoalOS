//go:build windows

package ipc

import (
	"context"
	"fmt"
	"net"
)

// transport_windows.go — Windows named pipe 传输实现（任务 1.7）。
//
// 契约（08 §17.4 + R-955 ACL 基线 + R-1013 安全防线）：
//   管道名=\\.\pipe\goalos-ipc-<GUID 小写 hex>（crypto/rand 16 字节——不可猜测，
//   STRIDE 0.6 缓解）；创建=CreateNamedPipeW（PIPE_ACCESS_DUPLEX|FILE_FLAG_OVERLAPPED；
//   PIPE_TYPE_BYTE|PIPE_READMODE_BYTE|PIPE_WAIT）；连接后首帧必须完成 §17.2 握手
//   （双 nonce 转录绑定，R-1148），否则 daemon 端 DisconnectNamedPipe+Fatal。
//   Client SQOS=SECURITY_IDENTIFICATION；Server=FIRST_PIPE_INSTANCE+REJECT_REMOTE。
//   代码基线：golang.org/x/sys/windows 直写（禁止无 SQOS 参数控制的封装库——
//   n.pipe.v2 不满足本规约）。CI 静态检查（check-windows-ipc-security.sh——
//   AST 扫描：连接 \\.\pipe\ 路径的 CreateFileW 调用必须含 SECURITY_IDENTIFICATION
//   常量，否则构建阻断）。

// WindowsTransport — named pipe 传输实现（Windows）。
type WindowsTransport struct {
	conn net.Conn // 已建立的 pipe 连接（握手完成后）
}

// NewWindowsTransport — 构造 named pipe 传输（daemon 侧：accept 后的连接）。
func NewWindowsTransport(conn net.Conn) *WindowsTransport {
	return &WindowsTransport{conn: conn}
}

// DialPipe — 子进程侧：连接 daemon 的 named pipe（管道名由 spawn 原语下发）。
// 契约：Client SQOS=SECURITY_IDENTIFICATION（R-1013 安全防线）。
func DialPipe(ctx context.Context, pipeName string) (*WindowsTransport, error) {
	// 骨架：拨号实现归任务 1.7 完成态（x/sys/windows 直写 CreateFileW）。
	_ = ctx
	return nil, fmt.Errorf("transport_windows.DialPipe: 骨架——实现归任务 1.7 完成态（x/sys/windows CreateFileW+SECURITY_IDENTIFICATION）")
}

// Send — 发送一帧（两行 HMAC 协议）。帧编码=protocol 层（encodeFrame，与 Unix 同语义）。
func (t *WindowsTransport) Send(ctx context.Context, msg *Message) error {
	if t.conn == nil {
		return fmt.Errorf("transport: 连接已关闭")
	}
	frame, err := encodeFrame(msg)
	if err != nil {
		return err
	}
	select {
	case <-ctx.Done():
		return ctx.Err()
	default:
	}
	_, err = t.conn.Write(frame)
	return err
}

// Recv — 接收一帧（阻塞至帧到达或 ctx 取消）。
func (t *WindowsTransport) Recv(ctx context.Context) (*Message, error) {
	if t.conn == nil {
		return nil, fmt.Errorf("transport: 连接已关闭")
	}
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	default:
	}
	return decodeFrame(t.conn)
}

// Close — 关闭传输通道（幂等）。
func (t *WindowsTransport) Close() error {
	if t.conn != nil {
		err := t.conn.Close()
		t.conn = nil
		return err
	}
	return nil
}
