// backend_mxc.go — MXC 可选后端集成（任务 7.2——R-958：MXC 检测+预留接口 stub）。
//
// 契约：MXC=可选后端集成（P1 可选增强）；ProcessContainer 调用不实现（R-958——
// 时间重分配给对抗测试+编译器余量）；MXC 检测=探测 wxc-exec 存在（v0.3.0 仅检测，
// R-958 stub）。
package sandbox

import (
	"context"
	"fmt"
)

// MXCBackend — MXC 可选沙箱后端（MXC 检测+预留接口 stub）。
// 契约：ProcessContainer 调用不实现（R-958——时间重分配给对抗测试+编译器余量）；
// MXC 检测=探测 wxc-exec 存在（v0.3.0 仅检测——stub）。
type MXCBackend struct{}

// Execute — MXC 后端执行（骨架：ProcessContainer 调用不实现——R-958 stub）。
// 契约：MXC 检测=探测 wxc-exec 存在；ProcessContainer 调用不实现（时间重分配给
// 对抗测试+编译器余量）。
func (b *MXCBackend) Execute(ctx context.Context, cmd *CommandSpec, profile CompiledProfile) (*Result, error) {
	if !profile.Compiled() {
		return nil, fmt.Errorf("mxc backend: CompiledProfile 零值非法（R-1106）——必须经 sandbox.Compile() 产出")
	}
	// 骨架：ProcessContainer 调用不实现（R-958——时间重分配给对抗测试+编译器余量）。
	return nil, fmt.Errorf("mxc backend: 骨架——ProcessContainer 调用不实现（R-958 stub——时间重分配给对抗测试+编译器余量）")
}

// Close — 后端关闭（幂等）。
func (b *MXCBackend) Close() error { return nil }
