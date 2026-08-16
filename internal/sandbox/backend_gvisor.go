//go:build linux && !xinchuang

package sandbox

import (
	"context"
	"fmt"
)

// backend_gvisor.go — gVisor 后端（I4，R-1078——会议 #193 威胁模型升级）。
//
// 契约：runsc 子进程后端（不链接进 daemon，供应链审查同 agentbox 标准 R-916）；
// KVM 探测优先、无 KVM 自动 systrap（SECCOMP_RET_TRAP，无需硬件虚拟化——
// 信创 4.19 内核同样可跑 I4 用户态内核）；不兼容 syscall seccomp 兜底；
// 自动降级链 L5→L4→L3→L2。

// GVisorBackend — gVisor 沙箱后端（I4 隔离层）。
type GVisorBackend struct {
	kvmAvailable bool // KVM 可用性（启动期探测）
	systrap      bool // systrap 模式（无 KVM 时自动启用）
}

// NewGVisorBackend — 构造 gVisor 后端（KVM 探测优先→systrap 兜底）。
func NewGVisorBackend() *GVisorBackend {
	return &GVisorBackend{
		kvmAvailable: probeKVM(),
		systrap:      !probeKVM(), // 无 KVM→systrap（SECCOMP_RET_TRAP）
	}
}

// probeKVM — KVM 可用性探测（骨架：/dev/kvm 存在性检查）。
func probeKVM() bool {
	// 骨架：/dev/kvm 存在性检查（实现归 3.19 完成态）
	return false // 保守：无 KVM→systrap
}

// Execute — gVisor 沙箱执行（骨架：调用 runsc 子进程——实现归 3.19 完成态）。
func (b *GVisorBackend) Execute(ctx context.Context, cmd *CommandSpec, profile CompiledProfile) (*Result, error) {
	if !profile.Compiled() {
		return nil, fmt.Errorf("gvisor: CompiledProfile 零值非法（R-1106）——必须经 Compile() 产出")
	}
	// 骨架：runsc 子进程调用归 3.19 完成态
	return nil, fmt.Errorf("gvisor: 骨架——runsc 子进程调用归任务 3.19 完成态（kvm=%v systrap=%v）", b.kvmAvailable, b.systrap)
}

// Close — 关闭后端（幂等）。
func (b *GVisorBackend) Close() error { return nil }
