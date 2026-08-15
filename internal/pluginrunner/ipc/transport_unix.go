//go:build !windows

package ipc

import (
	"context"
	"fmt"
	"os"
)

// transport_unix.go — Unix fd 继承收口（任务 1.6）。
//
// 契约（08 §17.4 + R-928 CLOEXEC 卫生 + R-1086 统一 spawn 原语四防线）：
//   FD3 继承=daemon 经 cmd.ExtraFiles 传递 fd 3 给子进程；session_secret 经 FD3 前导帧
//   交付（fd 继承非环境变量——不出现于 environ/ps/core dump、不默认继承子孙进程，
//   ForgeDock /proc/$PPID/environ 泄漏实证教训）。secret 注入责任方=PluginRunner 唯一。

// UnixTransport — fd 继承传输实现（Unix）。
type UnixTransport struct {
	r *os.File // 读端（daemon 侧：子进程 stdout；子进程侧：fd 3 读）
	w *os.File // 写端
}

// NewUnixTransport — 构造 fd 继承传输（daemon 侧）。
// r/w=已建立的 pipe 两端（由 spawn 原语经 ExtraFiles 传递写端给子进程）。
func NewUnixTransport(r, w *os.File) *UnixTransport {
	return &UnixTransport{r: r, w: w}
}

// ChildFD3 — 子进程侧：从 fd 3 构造传输（子进程入口调用）。
// 契约：fd 3 已由 daemon 经 ExtraFiles 传递（CLOEXEC 卫生已保证非预期 fd 关闭）。
func ChildFD3() (*UnixTransport, error) {
	f := os.NewFile(3, "goalos-ipc")
	if f == nil {
		return nil, fmt.Errorf("FD3 不可用——子进程未收到 fd 继承（spawn 原语未传递）")
	}
	return &UnixTransport{r: f, w: f}, nil
}

// Send — 发送一帧（两行 HMAC 协议：行1=HMAC hex，行2=JSON payload）。
// 帧编码=protocol 层（encodeFrame）——HMAC 签名由调用方注入（session_secret 经 FD3 前导帧交付，
// 派生 hmac_key=HMAC-SHA256(session_secret,"stdout-hmac:v1"，R-1261）；传输层只负责帧边界+顺序。
func (t *UnixTransport) Send(ctx context.Context, msg *Message) error {
	if t.w == nil {
		return fmt.Errorf("transport: 写端已关闭")
	}
	frame, err := encodeFrame(msg)
	if err != nil {
		return err
	}
	// ctx 取消检查（写前）
	select {
	case <-ctx.Done():
		return ctx.Err()
	default:
	}
	_, err = t.w.Write(frame)
	return err
}

// Recv — 接收一帧（阻塞至帧到达或 ctx 取消）。
func (t *UnixTransport) Recv(ctx context.Context) (*Message, error) {
	if t.r == nil {
		return nil, fmt.Errorf("transport: 读端已关闭")
	}
	// ctx 取消检查（读前）
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	default:
	}
	return decodeFrame(t.r)
}

// Close — 关闭传输通道（幂等）。
func (t *UnixTransport) Close() error {
	var err error
	if t.r != nil {
		err = t.r.Close()
		t.r = nil
	}
	if t.w != nil {
		if e := t.w.Close(); e != nil && err == nil {
			err = e
		}
		t.w = nil
	}
	return err
}
