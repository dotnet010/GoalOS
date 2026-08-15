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

// Detect — 启动期平台能力探测（骨架：当前平台实测+模拟开关应用）。
// 探测失败不阻断启动——诚实标注降级（R-929 内核能力模拟开关：探测不到=如实标注）。
func Detect() (*PlatformCapabilities, error) {
	caps := &PlatformCapabilities{
		KernelVersion: kernelVersion(),
	}
	// 模拟开关（R-929/R-960）：GOALOS_SANDBOX_SIMULATE_KERNEL 整包模拟
	if sim := os.Getenv("GOALOS_SANDBOX_SIMULATE_KERNEL"); sim != "" {
		caps.SimulatedKernel = sim
		caps.KernelVersion = sim
		// 模拟模式下能力按目标内核版本降级（骨架：4.19 信创目标=Landlock 不可用）
		if sim == "4.19" {
			caps.LandlockABI = 0
			caps.UserNamespace = false // 4.19 发行版可能禁用——保守标注
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
