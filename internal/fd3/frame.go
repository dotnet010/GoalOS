// frame.go——FD3 帧协议 v1（平台中立纯字节协议——设计=开发计划/fd3-broker-设计.md §五）。
// 长度前缀二进制帧（禁文本注入面）；双单向管道承载（R-1662 v2——读/写流独立句柄）。
package fd3

import (
	"encoding/binary"
	"fmt"
	"io"
)

// 操作码（v1 最小面——设计 §五）。
const (
	OpOpen     byte = 0x01 // 沙箱→宿主：请求打开目标（payload=host:port）
	OpData     byte = 0x02 // 双向：数据
	OpClose    byte = 0x03 // 双向：半关（写侧关闭）
	OpPing     byte = 0x7F // 探活
	OpOpenOK   byte = 0x81 // 宿主→沙箱：OPEN 接受
	OpOpenDeny byte = 0xC1 // 宿主→沙箱：OPEN 拒绝（payload=理由——治理留痕同源）
	OpPong     byte = 0xFF // 探活应答
)

// maxFramePayload 帧体上限（4MiB——防畸形长度前缀爆内存；治理面裁剪另行收紧）。
const maxFramePayload = 4 << 20

// Frame 单帧。
type Frame struct {
	Op      byte
	Payload []byte
}

// WriteFrame 写帧（[u32 长度][u8 op][payload]——长度=payload 长，不含头部）。
func WriteFrame(w io.Writer, f Frame) error {
	if len(f.Payload) > maxFramePayload {
		return fmt.Errorf("fd3: 帧体超限 %d > %d", len(f.Payload), maxFramePayload)
	}
	var hdr [5]byte
	binary.LittleEndian.PutUint32(hdr[:4], uint32(len(f.Payload)))
	hdr[4] = f.Op
	if _, err := w.Write(hdr[:]); err != nil {
		return err
	}
	if len(f.Payload) > 0 {
		if _, err := w.Write(f.Payload); err != nil {
			return err
		}
	}
	return nil
}

// ReadFrame 读帧（畸形长度=协议错误 fail-closed）。
func ReadFrame(r io.Reader) (Frame, error) {
	var hdr [5]byte
	if _, err := io.ReadFull(r, hdr[:]); err != nil {
		return Frame{}, err
	}
	n := binary.LittleEndian.Uint32(hdr[:4])
	if n > maxFramePayload {
		return Frame{}, fmt.Errorf("fd3: 畸形帧长度 %d（上限 %d）", n, maxFramePayload)
	}
	payload := make([]byte, n)
	if n > 0 {
		if _, err := io.ReadFull(r, payload); err != nil {
			return Frame{}, err
		}
	}
	return Frame{Op: hdr[4], Payload: payload}, nil
}
