// frame_test.go——FD3 帧协议编解码测试（通用单测车道——R-1695-2「分层纳管」：
// 协议与帧解包测试=零 OS 依赖，随常规 PR/CI 三平台全量触发）。
//
// 无 build tag=刻意：帧层纯字节协议（frame.go 无平台面），任何平台任何 runner
// 均可全量执行——不 bind socket、不落文件系统路径、不依赖 sun_path 预算。
package fd3

import (
	"bytes"
	"encoding/binary"
	"io"
	"testing"
)

// allOps 全部操作码（v1 面——帧编解码须对每个 op 字节透明）。
var allOps = []byte{OpOpen, OpData, OpClose, OpPing, OpOpenOK, OpOpenDeny, OpPong}

// TestFD3Frame_RoundtripAllOps 全 op 编解码往返：op 字节+payload 逐字节保真。
func TestFD3Frame_RoundtripAllOps(t *testing.T) {
	for _, op := range allOps {
		payload := []byte{0x00, 0xFF, 'h', 'o', 's', 't', ':', '8', '0'} // 含 NUL/高位字节（禁文本注入面）
		var buf bytes.Buffer
		if err := WriteFrame(&buf, Frame{Op: op, Payload: payload}); err != nil {
			t.Fatalf("op=0x%02X 写帧失败: %v", op, err)
		}
		got, err := ReadFrame(&buf)
		if err != nil {
			t.Fatalf("op=0x%02X 读帧失败: %v", op, err)
		}
		if got.Op != op {
			t.Errorf("op 失真: 写入 0x%02X 读回 0x%02X", op, got.Op)
		}
		if !bytes.Equal(got.Payload, payload) {
			t.Errorf("op=0x%02X payload 失真: 写入 %q 读回 %q", op, payload, got.Payload)
		}
		if buf.Len() != 0 {
			t.Errorf("op=0x%02X 读帧后残留 %d 字节（越读）", op, buf.Len())
		}
	}
}

// TestFD3Frame_EmptyAndBoundaryPayload 空体/边界体：空 payload=合法帧（长度 0 不写体）；
// 上限正好=maxFramePayload 可编码；超限=写侧 fail-closed 拒绝。
func TestFD3Frame_EmptyAndBoundaryPayload(t *testing.T) {
	// 空 payload（长度 0——体不写，仅 5 字节头）
	var buf bytes.Buffer
	if err := WriteFrame(&buf, Frame{Op: OpOpenOK}); err != nil {
		t.Fatalf("空体写帧失败: %v", err)
	}
	if buf.Len() != 5 {
		t.Errorf("空体帧应恰为 5 字节头，实得 %d", buf.Len())
	}
	got, err := ReadFrame(&buf)
	if err != nil {
		t.Fatalf("空体读帧失败: %v", err)
	}
	if got.Op != OpOpenOK || len(got.Payload) != 0 {
		t.Errorf("空体往返失真: op=0x%02X payload=%q", got.Op, got.Payload)
	}

	// 边界：正好上限（可编码可解码——上限为闭区间）
	full := bytes.Repeat([]byte{'x'}, maxFramePayload)
	if err := WriteFrame(&buf, Frame{Op: OpData, Payload: full}); err != nil {
		t.Fatalf("上限体（%d 字节）应可编码: %v", maxFramePayload, err)
	}
	gotFull, err := ReadFrame(&buf)
	if err != nil {
		t.Fatalf("上限体读帧失败: %v", err)
	}
	if len(gotFull.Payload) != maxFramePayload {
		t.Errorf("上限体长度失真: %d ≠ %d", len(gotFull.Payload), maxFramePayload)
	}

	// 超限：写侧拒绝（不落畸形帧进管道——fail-closed）
	over := bytes.Repeat([]byte{'x'}, maxFramePayload+1)
	if err := WriteFrame(&buf, Frame{Op: OpData, Payload: over}); err == nil {
		t.Error("超限帧体应被写侧拒绝（fail-closed），实际通过")
	}
}

// TestFD3Frame_MalformedLengthFailClosed 畸形长度前缀=协议错误拒绝（防长度前缀爆内存：
// 拒绝须发生在分配之前——不断言分配行为，只断言返回值与不读出内容）。
func TestFD3Frame_MalformedLengthFailClosed(t *testing.T) {
	// 长度前缀 > 上限（未随体——正是攻击形态：谎报长度）
	var b bytes.Buffer
	var hdr [5]byte
	binary.LittleEndian.PutUint32(hdr[:4], maxFramePayload+1)
	hdr[4] = OpData
	b.Write(hdr[:])
	if _, err := ReadFrame(&b); err == nil {
		t.Error("畸形长度前缀应被拒绝（fail-closed），实际通过")
	}

	// 头截断（不足 5 字节）
	if _, err := ReadFrame(bytes.NewReader([]byte{0x01, 0x00})); err != io.ErrUnexpectedEOF {
		t.Errorf("截断头应返回 ErrUnexpectedEOF，实得 %v", err)
	}

	// 体截断（声明 8 字节实给 3 字节）
	var b2 bytes.Buffer
	binary.LittleEndian.PutUint32(hdr[:4], 8)
	hdr[4] = OpData
	b2.Write(hdr[:])
	b2.Write([]byte("abc"))
	if _, err := ReadFrame(&b2); err != io.ErrUnexpectedEOF {
		t.Errorf("截断体应返回 ErrUnexpectedEOF，实得 %v", err)
	}

	// 空流（连接已关——EOF 非畸形，调用方按断连处置）
	if _, err := ReadFrame(bytes.NewReader(nil)); err != io.EOF {
		t.Errorf("空流应返回 EOF，实得 %v", err)
	}
}

// TestFD3Frame_StreamSequence 流式定界：连写多帧=逐帧按序解出（含空体帧夹缝——
// 定界不越读不错位，泵路径 32KiB 分块语义的编码侧前提）。
func TestFD3Frame_StreamSequence(t *testing.T) {
	want := []Frame{
		{Op: OpOpen, Payload: []byte("127.0.0.1:8080")},
		{Op: OpData}, // 空体夹缝
		{Op: OpData, Payload: []byte("payload-1")},
		{Op: OpClose},
	}
	var buf bytes.Buffer
	for i, f := range want {
		if err := WriteFrame(&buf, f); err != nil {
			t.Fatalf("第 %d 帧写入失败: %v", i, err)
		}
	}
	for i, w := range want {
		got, err := ReadFrame(&buf)
		if err != nil {
			t.Fatalf("第 %d 帧读出失败: %v", i, err)
		}
		if got.Op != w.Op || !bytes.Equal(got.Payload, w.Payload) {
			t.Errorf("第 %d 帧错位: 期望 op=0x%02X payload=%q，实得 op=0x%02X payload=%q",
				i, w.Op, w.Payload, got.Op, got.Payload)
		}
	}
	if buf.Len() != 0 {
		t.Errorf("序列解完应零残留，实得 %d 字节", buf.Len())
	}
}
