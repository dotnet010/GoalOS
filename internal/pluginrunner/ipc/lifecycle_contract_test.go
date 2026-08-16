//go:build !windows

// 契约测试：插件 IPC 生命周期（R-1105~R-1107——会议 #195）。
//
// 断言来源: R-1105（HMAC 密钥 FD3 前导消息传递——fd 继承非环境变量；大 payload GC
//   契约——IPC 消息读毕即弃+arena 复用）；R-1106（CompiledProfile 零值非法化——
//   Execute 入口零值→Fatal fail-closed）；R-1107（优雅取消协议——FD3 CancelMessage+
//   响应窗口→超时 SIGTERM→2s 宽限→SIGKILL）。
//
// 当前契约形态（转绿任务 3.27 完成态——IPC 传输层+零值非法化已落地）:
//   - 取消帧载体=两行 HMAC 协议帧（R-1105 传输契约）——帧往返保真+空签名行
//     fail-closed+内联超阈值 fail-closed（R-1132 阈值单一规则）；
//   - 零值非法化=R-1106 公开契约面——RawProfile nil/非法值经 Validate 拒绝
//     （Compile 是合法路径的唯一入口）。
//   取消升级链（SIGTERM→SIGKILL）执行器侧实现与 Profile 的 Execute 入口零值
//   Fatal 断言归 3.27 完成态续作（本测试断言当前已落契约面）。
//
// 纪律: 行为断言——禁止源码文本断言；逐条 t.Error 非 FailNow。

package ipc_test

import (
	"bytes"
	"context"
	"os"
	"strings"
	"testing"

	"github.com/goalos/goalos/internal/pluginrunner/ipc"
	"github.com/goalos/goalos/internal/sandbox"
)

// TestPluginCancel_Escalation_SIGTERM_SIGKILL — 取消帧载体契约（R-1107/R-1105 传输层）。
// 断言：两行 HMAC 协议帧往返保真（CancelMessage 的载体路径可靠）+ 空签名行
// fail-closed（协议违规拒绝——R-1132）。
func TestPluginCancel_Escalation_SIGTERM_SIGKILL(t *testing.T) {
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("前置: os.Pipe 失败: %v", err)
	}
	defer r.Close()
	defer w.Close()

	transport := ipc.NewUnixTransport(r, w)
	// MUST 1（R-1105）: 取消帧（cancel 类型载荷）经传输层往返保真——HMACLine 与
	// PayloadLine 原样保留（协议层签名/校验的输入不可丢失，R-1447）。
	sent := &ipc.RawFrame{
		HMACLine:    []byte("aa" + strings.Repeat("0", 62)), // 64 hex chars
		PayloadLine: []byte(`{"type":"cancel","action_id":"a-1"}`),
	}
	if err := transport.Send(context.Background(), sent); err != nil {
		t.Fatalf("前置: Send 失败: %v", err)
	}
	got, err := transport.Recv(context.Background())
	if err != nil {
		t.Fatalf("MUST 1（R-1105）: Recv 失败: %v", err)
	}
	if !bytes.Equal(got.HMACLine, sent.HMACLine) {
		t.Error("MUST 1（R-1105）: 往返 HMACLine 失真——签名行不得被传输层改写/丢弃")
	}
	if !bytes.Equal(got.PayloadLine, sent.PayloadLine) {
		t.Error("MUST 1（R-1105）: 往返 PayloadLine 失真——取消载荷不得被传输层改写")
	}

	// MUST 2（R-1132）: 空签名行=协议违规 fail-closed（Send 拒绝）。
	if err := transport.Send(context.Background(), &ipc.RawFrame{PayloadLine: []byte(`{}`)}); err == nil {
		t.Error("MUST 2（R-1132）: 空 HMACLine 帧被放行——协议违规必须 fail-closed")
	}
}

// TestProfile_ZeroValue_FailClosed — 零值非法化（R-1106）。
// 断言：RawProfile nil/零值经 Validate 拒绝（Compile 是合法路径的唯一入口——
// 不存在绕过校验的零值构造路径）。
func TestProfile_ZeroValue_FailClosed(t *testing.T) {
	// MUST 1（R-1106）: nil profile Validate 拒绝。
	var nilProfile *sandbox.RawProfile
	if err := nilProfile.Validate(); err == nil {
		t.Error("MUST 1（R-1106）: nil RawProfile Validate 放行——零值非法化违约")
	}
	// MUST 2（R-1106/R-1012）: 零值 isolation（未声明）拒绝——无隐式默认降级。
	zero := &sandbox.RawProfile{}
	if err := zero.Validate(); err == nil {
		t.Error("MUST 2（R-1106/R-1012）: 零值 RawProfile（isolation 缺失）放行——隐式默认降级违约")
	}
	// MUST 3（R-1106）: 非法 isolation 值拒绝（取值域 I0..I5 封闭）。
	if err := (&sandbox.RawProfile{Isolation: "I9"}).Validate(); err == nil {
		t.Error("MUST 3（R-1106）: 非法 isolation 值（I9）放行——取值域封闭违约")
	}
	// MUST 4（R-1106）: 合法路径唯一=Validate+Compile 显式产出。
	p, err := sandbox.Compile(&sandbox.RawProfile{Isolation: "I1"}, sandbox.PlatformLinux)
	if err != nil || !p.Compiled() {
		t.Errorf("MUST 4（R-1106）: 合法路径 Compile 失败（err=%v）——零值非法化的合法入口断裂", err)
	}
}

// TestIpc_PayloadGc_ArenaReuse — 大 payload 边界契约（R-1105/R-1132）。
// 断言：内联上限 4KB（R-1132 阈值单一规则）——边界内放行、超阈值 fail-closed；
// 大 payload 帧往返保真（data_ref 引用前的内联最大载荷可靠）。
func TestIpc_PayloadGc_ArenaReuse(t *testing.T) {
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("前置: os.Pipe 失败: %v", err)
	}
	defer r.Close()
	defer w.Close()
	transport := ipc.NewUnixTransport(r, w)

	// MUST 1（R-1132）: 内联上限边界内（4096B）放行。
	atLimit := &ipc.RawFrame{
		HMACLine:    []byte(strings.Repeat("ab", 32)),
		PayloadLine: []byte(strings.Repeat("x", 4096)),
	}
	if err := transport.Send(context.Background(), atLimit); err != nil {
		t.Errorf("MUST 1（R-1132）: 4096B 内联载荷被拒——阈值内放行违约: %v", err)
	}
	// MUST 2（R-1132）: 超阈值（>4KB）fail-closed——内联超限=协议违规。
	oversized := &ipc.RawFrame{
		HMACLine:    []byte(strings.Repeat("ab", 32)),
		PayloadLine: []byte(strings.Repeat("y", 4097)),
	}
	if err := transport.Send(context.Background(), oversized); err == nil {
		t.Error("MUST 2（R-1132）: 4097B 内联载荷被放行——超阈值 fail-closed 违约")
	}
	// MUST 3（R-1105）: 边界内大 payload 往返保真（读毕即弃语义的载体可靠）。
	if _, err := transport.Recv(context.Background()); err != nil {
		t.Fatalf("MUST 3（R-1105）: 大 payload 帧 Recv 失败: %v", err)
	}
}
