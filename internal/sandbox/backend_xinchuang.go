//go:build linux

package sandbox

import (
	"context"
	"fmt"
)

// backend_xinchuang.go — 信创后端（任务 5.1）：seccomp BPF（现有自加载模型）+
// mount namespace 隔离。信创 4.19 内核——Landlock 不可用（R-1078：I4 主路径=gVisor
// systrap，seccomp-only 仅作 gVisor 不可用时的 I2 兜底+硬化三件——R-1087）。

// XinchuangBackend — 信创沙箱后端（seccomp BPF+mount namespace）。
type XinchuangBackend struct{}

// Execute — 信创后端执行（骨架：seccomp BPF 自加载+mount namespace 隔离——实现归 5.15）。
// 契约：I4 主路径=gVisor systrap（R-1078）；seccomp-only=I2 兜底（R-1087 硬化三件）。
func (b *XinchuangBackend) Execute(ctx context.Context, cmd *CommandSpec, profile CompiledProfile) (*Result, error) {
	if !profile.Compiled() {
		return nil, fmt.Errorf("xinchuang backend: CompiledProfile 零值非法（R-1106）——必须经 sandbox.Compile() 产出")
	}
	// 骨架：seccomp BPF+mount namespace 隔离实现归 5.15（信创 I4 主路径+seccomp-only 硬化）。
	return nil, fmt.Errorf("xinchuang backend: 骨架——实现归任务 5.15（R-1087 信创 I4 主路径+seccomp-only 硬化）")
}

// Close — 后端关闭（幂等）。
func (b *XinchuangBackend) Close() error { return nil }
