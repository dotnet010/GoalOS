//go:build darwin

package sandbox

import (
	"context"
	"fmt"
)

// backend_darwin.go — macOS 后端（任务 5.3）：sandbox-exec 统一收口（现有 goalos-*.sb
// 纳入接口）。macOS 专业平台——Seatbelt 路径（R-1399 darwin-seatbelt 命名）。

// DarwinBackend — macOS 沙箱后端（sandbox-exec/Seatbelt）。
type DarwinBackend struct{}

// Execute — macOS 后端执行（骨架：sandbox-exec 统一收口——现有 goalos-*.sb 纳入接口）。
// 契约：Seatbelt 路径（R-1399 darwin-seatbelt）；无内核特性探测需求。
func (b *DarwinBackend) Execute(ctx context.Context, cmd *CommandSpec, profile CompiledProfile) (*Result, error) {
	if !profile.Compiled() {
		return nil, fmt.Errorf("darwin backend: CompiledProfile 零值非法（R-1106）——必须经 sandbox.Compile() 产出")
	}
	// 骨架：sandbox-exec 统一收口实现归 5.3 完成态。
	return nil, fmt.Errorf("darwin backend: 骨架——实现归任务 5.3 完成态（sandbox-exec 统一收口）")
}

// Close — 后端关闭（幂等）。
func (b *DarwinBackend) Close() error { return nil }
