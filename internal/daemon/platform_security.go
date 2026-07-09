// Package daemon — v0.2.0 Week 1 S4: 平台安全级别标注
package daemon

import (
	"fmt"
	"runtime"
)

// SecurityLevel 是平台安全隔离级别。
type SecurityLevel struct {
	Platform     string // "darwin" | "linux"
	Level        string // "L1" | "L2" (R-829 统一)
	Label        string // 用户可见标签
	Capabilities []string
}

// GetPlatformSecurityLevel 返回当前平台的安全隔离级别（R-829 统一 L2）。
// Linux:   L2（基础系统隔离 + 子进程自声明 seccomp——daemon 侧无验证）
// macOS:   L2（基础系统隔离——无系统调用过滤。v0.3.0 App Sandbox）
// Windows: L1（基础系统隔离——无系统调用过滤）
func GetPlatformSecurityLevel() SecurityLevel {
	if runtime.GOOS == "darwin" {
		return SecurityLevel{
			Platform: "darwin",
			Level:    "L2",
			// S4: macOS 安全降级诚实标注
			Label: "当前平台安全隔离级别：基础（不含系统调用过滤）",
			Capabilities: []string{
				"独立 PID（Setpgid）——OS 强制",
				"文件权限 0700——OS 强制",
				"网络隔离——未强制（代码约定，攻击者可绕过）。v0.3.0 App Sandbox",
				"fork/execve 限制——未强制（代码约定，攻击者可绕过）。v0.3.0 App Sandbox",
				"setuid/ptrace 限制——未强制（代码约定，攻击者可绕过）。v0.3.0 App Sandbox",
			},
		}
	}
	if runtime.GOOS == "linux" {
		return SecurityLevel{
			Platform: "linux",
			Level:    "L2",
			Label:    "当前平台安全隔离级别：基础（系统隔离 + 子进程沙箱）",
			Capabilities: []string{
				"seccomp BPF——子进程自加载（daemon 侧无强制验证）",
				"CLONE_NEWNET 网络隔离——OS 强制",
				"文件权限 0700——OS 强制",
				"fork/execve/setuid/ptrace 限制——seccomp 强制",
			},
		}
	}
	// G13: Windows 等非 macOS/Linux 平台——诚实标注安全降级
	return SecurityLevel{
		Platform: runtime.GOOS,
		Level:    "L1",
		Label:    fmt.Sprintf("当前平台（%s）安全隔离级别：基础（无系统调用过滤）", runtime.GOOS),
		Capabilities: []string{
			"文件权限 0700——OS 强制",
			"进程隔离——代码约定",
		},
	}
}
