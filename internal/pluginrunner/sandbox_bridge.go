package pluginrunner

import (
	"context"
	"fmt"

	"github.com/goalos/goalos/internal/sandbox"
)

// sandbox_bridge.go — Plugin Runner 接入 Sandbox.Execute（任务 1.11）。
//
// 契约（R-1111 组件边界+R-906a 治理不变量）：Sandbox=执行环境构造，PluginRunner=插件
// 生命周期管理；本文件=ExecuteMessage（IPC 协议层）→CommandSpec（Sandbox 层）的翻译点。
// 治理顺序：调用链=PipelineRunner→五引擎→PluginRunner→sandbox.Execute——本桥接
// 只在 Governance 五引擎全部通过后调用（调用方保证，本桥不接治理）。

// TranslateToCommandSpec — ExecuteMessage（ActionRequest）→CommandSpec 翻译。
// 契约（R-988）：不暴露 *exec.Cmd；Env 注入=${secret:xxx} 解密后注入（R-917 红线——
// secret 注入责任方=PluginRunner 唯一，本桥接是注入点）；WorkingDir=profile workspace。
func TranslateToCommandSpec(action ActionRequest, workspace string) (*sandbox.CommandSpec, error) {
	if action.Target == "" {
		return nil, fmt.Errorf("sandbox bridge: action.Target 为空——可执行文件路径缺失")
	}
	spec := &sandbox.CommandSpec{
		Path:       action.Target,
		Args:       nil, // 参数归 Action 定义（骨架：nil=无参数，由调用方填充）
		Env:        nil, // R-917：Env 注入=secret 解密后注入——骨架：nil，由 PluginRunner 在授权后填充
		WorkingDir: workspace,
		Stdio:      sandbox.StdioCapture,                     // Goal 模式默认捕获（数据通道目录，R-951）
		Resources:  sandbox.ResourceLimit{ProcessTree: true}, // R-957 进程树范围
		Timeout:    0,                                        // 进程树超时由 profile max_execution_time 透传（骨架：0=不限，由调用方设置）
	}
	return spec, nil
}

// ExecuteSandboxed — Goal 模式 Windows 插件首次沙箱化（任务 1.11）：
// ActionRequest→CommandSpec 翻译→sandbox.Execute 调用。profile 必须经 Compile() 产出
// （R-1106 零值非法化——零值 profile 在 Execute 入口被 Fatal fail-closed 拒绝）。
func ExecuteSandboxed(ctx context.Context, sb sandbox.Sandbox, action ActionRequest, workspace string, profile sandbox.CompiledProfile) (*sandbox.Result, error) {
	if !profile.Compiled() {
		return nil, fmt.Errorf("sandbox bridge: CompiledProfile 零值非法（R-1106）——必须经 sandbox.Compile() 产出")
	}
	spec, err := TranslateToCommandSpec(action, workspace)
	if err != nil {
		return nil, err
	}
	result, err := sb.Execute(ctx, spec, profile)
	if err != nil {
		// R-973 error 语义边界：error=沙箱基础设施失败（Fatal/四态），Result=nil
		return nil, fmt.Errorf("sandbox bridge: 沙箱基础设施失败: %w", err)
	}
	return result, nil
}
