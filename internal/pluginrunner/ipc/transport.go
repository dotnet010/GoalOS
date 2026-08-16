// Package ipc — v0.3.0 插件协议传输层（R-922）：FD3 控制通道 + 两行 HMAC 协议。
//
// 传输层职责（任务 1.3）：Transport 接口统一 fd 继承（Unix）与 named pipe（Windows）两种
// 底层传输的语义——消息边界/顺序/HMAC 完整性/取消帧。与 pluginrunner 的 ExecuteMessage
// 关系：ExecuteMessage（协议层）→ 翻译 → Transport 帧（传输层）。
//
// 契约（08 §17）：握手协议（双 nonce 转录绑定+5s 超时，R-1148）；消息头 extensions 预留
// 字段（R-1025——分类器等后续模块追加字段不破坏 v0.3 协议兼容，transport 层原样透传）；
// 抗重放（seq 单调，重放帧丢弃——TestHandshake_ReplayDropped）；大 payload GC（R-1105）。
package ipc

import (
	"context"
)

// Message — 传输层帧（两行 HMAC 协议载荷）。
type Message struct {
	Type       string            // 消息类型（execute/result/cancel/progress/...）
	Seq        uint64            // 抗重放序号（单调递增）
	Payload    []byte            // JSON payload（<4KB 内联；超限→data_ref 引用）
	Extensions map[string]string // R-1025 预留字段——transport 层原样透传
}

// Transport — 传输层接口（任务 1.3 骨架）。fd 继承（Unix）与 named pipe（Windows）同语义。
type Transport interface {
	// Send — 发送一帧（HMAC 完整性由实现层保证）。
	Send(ctx context.Context, rf *RawFrame) error
	// Recv — 接收一帧（阻塞至帧到达或 ctx 取消）。
	Recv(ctx context.Context) (*RawFrame, error)
	// Close — 关闭传输通道（幂等）。
	Close() error
}

// HandshakeResult — 握手结果（双 nonce 转录绑定，R-1148）。
type HandshakeResult struct {
	PeerNonce []byte // 对端 nonce
	Bound     bool   // 转录绑定完成
}

// Handshake — 传输层握手（双 nonce 转录绑定+5s 超时）。
// 骨架：接口定义+返回骨架；实现归 transport_unix.go/transport_windows.go（任务 1.6/1.7）。
type Handshaker interface {
	Handshake(ctx context.Context, t Transport) (*HandshakeResult, error)
}

// 契约测试锚点（12清单 C 表）：TestTransport_MessageParity（同一消息两传输语义一致）/
// TestTransport_ExtensionsPassThrough（extensions 原样透传）/TestTransport_FdCloseOnExec/
// TestTransport_NoSecretInEvent。
