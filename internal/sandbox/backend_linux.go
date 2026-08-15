//go:build linux

package sandbox

import (
	"context"
	"fmt"
)

// backend_linux.go — 现代 Linux 收口（任务 5.4）：与信创后端共用 BPF 核心，
// namespace 策略分层。现代 Linux（>=5.13——Landlock 可用）。

// LinuxBackend — 现代 Linux 沙箱后端（seccomp BPF+mount namespace+Landlock 5.13+）。
type LinuxBackend struct{}

// Execute — 现代 Linux 后端执行（骨架：seccomp BPF+mount namespace+Landlock——实现归 5.4 完成态）。
// 契约：与信创后端共用 BPF 核心；namespace 策略分层；Landlock 5.13+ 文件禁闭。
func (b *LinuxBackend) Execute(ctx context.Context, cmd *CommandSpec, profile CompiledProfile) (*Result, error) {
	if !profile.Compiled() {
		return nil, fmt.Errorf("linux backend: CompiledProfile 零值非法（R-1106）——必须经 sandbox.Compile() 产出")
	}
	// 骨架：现代 Linux 收口实现归 5.4 完成态（与信创后端共用 BPF 核心+namespace 策略分层）。
	return nil, fmt.Errorf("linux backend: 骨架——实现归任务 5.4 完成态（现代 Linux 收口）")
}

// Close — 后端关闭（幂等）。
func (b *LinuxBackend) Close() error { return nil }
