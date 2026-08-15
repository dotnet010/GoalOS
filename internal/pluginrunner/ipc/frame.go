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

// RawFrame — 传输层与协议层的边界类型（R-1447——发现 1+5 合并：encode/decode 责任下放对称化）。
// 传输层只搬运原始字节行（hmacLine+payloadLine 原样），不生成伪占位、不丢弃校验输入；
// 协议层负责签名/校验/反序列化。
type RawFrame struct {
	HMACLine    []byte // 行1=HMAC hex 签名（协议层 VerifyHMAC 的输入——不可丢失）
	PayloadLine []byte // 行2=JSON payload（<4KB 内联；超限→data_ref 引用）
}

// encodeFrame — RawFrame→两行 HMAC 协议帧（拼接两行原样——HMACLine 由调用方填入，
// 传输层不生成伪占位）。契约：HMACLine 非空（协议违规 fail-closed——R-1132）。
func encodeFrame(rf *RawFrame) ([]byte, error) {
	if rf == nil {
		return nil, fmt.Errorf("frame: RawFrame 为 nil")
	}
	if len(rf.HMACLine) == 0 {
		return nil, fmt.Errorf("frame: HMACLine 为空——协议违规 fail-closed（R-1132：签名行必须由协议层注入）")
	}
	if len(rf.PayloadLine) > maxInlinePayload {
		return nil, fmt.Errorf("frame: 内联超阈值（%d > %d）——协议违规 fail-closed（R-1132）", len(rf.PayloadLine), maxInlinePayload)
	}
	return []byte(string(rf.HMACLine) + "\n" + string(rf.PayloadLine) + "\n"), nil
}

// decodeFrame — 两行 HMAC 协议帧→RawFrame（读两行原样——HMACLine 不丢弃，交协议层校验）。
// 消息反序列化归 protocol 层（VerifyHMAC 通过后 Unmarshal）——传输层职责收窄=只搬运字节行。
func decodeFrame(r io.Reader) (*RawFrame, error) {
	br := bufio.NewReader(r)
	hmacLine, err := br.ReadBytes('\n')
	if err != nil {
		return nil, fmt.Errorf("frame: 读 HMAC 行失败: %w", err)
	}
	payloadLine, err := br.ReadBytes('\n')
	if err != nil {
		return nil, fmt.Errorf("frame: 读 payload 行失败: %w", err)
	}
	// 去除行尾 \n（两行协议的边界符）
	rf := &RawFrame{
		HMACLine:    hmacLine[:len(hmacLine)-1],
		PayloadLine: payloadLine[:len(payloadLine)-1],
	}
	return rf, nil
}

// UnmarshalPayload — RawFrame.PayloadLine→Message（协议层调用——VerifyHMAC 通过后）。
func UnmarshalPayload(rf *RawFrame) (*Message, error) {
	var msg Message
	if err := json.Unmarshal(rf.PayloadLine, &msg); err != nil {
		return nil, fmt.Errorf("frame: JSON 反序列化失败: %w", err)
	}
	return &msg, nil
}
