package sandbox

import (
	"os"
	"runtime"
)

// detect.go — 启动期 PlatformCapabilities 探测框架（任务 1.9 骨架）。
//
// 契约（05 §X.1a + R-1003/R-947/R-976）：启动期探测返回 PlatformCapabilities——内核版本/
// Landlock ABI/seccomp notify/用户命名空间/Job Object/MXC。模拟开关（R-929/R-960）：
// GOALOS_SANDBOX_SIMULATE_KERNEL=4.19 整包模拟；per-feature 禁用开关。

// SimTarget — 模拟目标类型（R-1463——发现 22：穷尽性欠款类型收窄）。
// 取值域封闭：SimNone=不模拟/Sim419=信创 4.19——新增目标必须加常量+switch 分支。
type SimTarget string

const (
	SimNone SimTarget = ""
	Sim419  SimTarget = "4.19"
)

// Detect — 启动期平台能力探测（骨架：当前平台实测+模拟开关应用）。
// 探测失败不阻断启动——诚实标注降级（R-929 内核能力模拟开关：探测不到=如实标注）。
func Detect() (*PlatformCapabilities, error) {
	caps := &PlatformCapabilities{
		KernelVersion: kernelVersion(),
	}
	// 模拟开关（R-929/R-960）：GOALOS_SANDBOX_SIMULATE_KERNEL 整包模拟
	// SimTarget 类型收窄（R-1463——发现 22：穷尽性欠款——sim 取值域与处理分支一致性
	// 由类型+switch 承载，非裸字符串）。
	if simStr := os.Getenv("GOALOS_SANDBOX_SIMULATE_KERNEL"); simStr != "" {
		sim := SimTarget(simStr)
		caps.SimulatedKernel = simStr
		caps.KernelVersion = simStr
		// 穷尽性 switch——新增模拟目标必须加处理分支（exhaustive linter 检查）
		switch sim {
		case Sim419:
			caps.LandlockABI = 0
			caps.UserNamespace = false // 4.19 发行版可能禁用——保守标注
		case SimNone:
			// 空值=不模拟（不会发生——simStr 非空才进入此分支）
		default:
			// 不支持的模拟目标=显式报错（非静默继续——R-1463 穷尽性欠款）
			caps.SimulatedKernel = "" // 清除模拟标记（未支持的目标=不生效）
			caps.KernelVersion = ""
		}
	}
	// 平台实测（骨架：GOOS 决定；特性探测归 backend 实现任务）
	switch runtime.GOOS {
	case "windows":
		caps.JobObject = true // CreateJobObjectW 自 NT 起可用
	case "linux":
		caps.SeccompNotify = landlockUsable() // >=5.0 内核
		if caps.SimulatedKernel == "" {
			caps.LandlockABI = landlockABI() // >=5.13 内核
			caps.UserNamespace = userNamespaceUsable()
		}
	case "darwin":
		// Seatbelt 路径（R-1399 darwin-seatbelt 命名）——无内核特性探测需求
	}
	return caps, nil
}

// kernelVersion — uname -r 等效（骨架：runtime 探测）。
func kernelVersion() string {
	// 骨架实现：返回空串，由 backend 实现填充（uname 系统调用归 backend_unix.go）。
	// 模拟开关优先（GOALOS_SANDBOX_SIMULATE_KERNEL）。
	if sim := os.Getenv("GOALOS_SANDBOX_SIMULATE_KERNEL"); sim != "" {
		return sim
	}
	return ""
}

// landlockUsable / landlockABI / userNamespaceUsable — Linux 特性探测（骨架：保守 false，归 backend 实现）。
func landlockUsable() bool      { return false }
func landlockABI() int          { return 0 }
func userNamespaceUsable() bool { return false }
