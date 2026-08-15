package ipc

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
)

// frame.go — 帧编解码（两行 HMAC 协议载荷的传输层封装）。
//
// 契约（08 §17.1 + R-955）：行1=HMAC hex 签名（调用方注入，传输层不感知密钥），
// 行2=JSON payload（<4KB 内联；超限→data_ref 引用——内联超阈值=协议违规 fail-closed，
// R-1132/R-1149）。extensions 预留字段原样透传（R-1025）。抗重放=seq 单调（重放帧丢弃，
// TestHandshake_ReplayDropped）。

// maxInlinePayload — 内联 payload 上限（<4KB——R-1132 阈值单一规则）。
const maxInlinePayload = 4096

// encodeFrame — Message→两行 HMAC 协议帧（行1=HMAC 占位由调用方填，行2=JSON）。
// 骨架：JSON 序列化+长度校验；HMAC 签名由 pluginrunner.fd3_protocol 注入。
func encodeFrame(msg *Message) ([]byte, error) {
	if msg == nil {
		return nil, fmt.Errorf("frame: 消息为 nil")
	}
	payload, err := json.Marshal(msg)
	if err != nil {
		return nil, fmt.Errorf("frame: JSON 序列化失败: %w", err)
	}
	if len(payload) > maxInlinePayload {
		return nil, fmt.Errorf("frame: 内联超阈值（%d > %d）——协议违规 fail-closed（R-1132）", len(payload), maxInlinePayload)
	}
	// 两行协议：行1=HMAC hex（占位，调用方签名后覆盖），行2=JSON payload
	return []byte("\n" + string(payload) + "\n"), nil
}

// decodeFrame — 两行 HMAC 协议帧→Message（读两行：行1=HMAC，行2=JSON）。
func decodeFrame(r io.Reader) (*Message, error) {
	br := bufio.NewReader(r)
	// 行1=HMAC hex（传输层不校验——校验归 protocol 层 VerifyHMAC）
	if _, err := br.ReadBytes('\n'); err != nil {
		return nil, fmt.Errorf("frame: 读 HMAC 行失败: %w", err)
	}
	// 行2=JSON payload
	line, err := br.ReadBytes('\n')
	if err != nil {
		return nil, fmt.Errorf("frame: 读 payload 行失败: %w", err)
	}
	var msg Message
	if err := json.Unmarshal(line, &msg); err != nil {
		return nil, fmt.Errorf("frame: JSON 反序列化失败: %w", err)
	}
	return &msg, nil
}
