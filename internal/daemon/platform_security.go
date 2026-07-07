// Package daemon — v0.2.0 Week 1 S4: 平台安全级别标注
package daemon

import "runtime"

// SecurityLevel 是平台安全隔离级别。
type SecurityLevel struct {
	Platform     string // "darwin" | "linux"
	Level        string // "L2" | "L3"
	Label        string // 用户可见标签
	Capabilities []string
}

// GetPlatformSecurityLevel 返回当前平台的安全隔离级别（S4）。
// macOS: L2（基础隔离——独立PID+0700，不含系统调用过滤）
// Linux: L3（完整隔离——seccomp+namespace+文件权限）
func GetPlatformSecurityLevel() SecurityLevel {
	if runtime.GOOS == "darwin" {
		return SecurityLevel{
			Platform: "darwin",
			Level:    "L2",
			// S4: macOS 安全降级诚实标注
			Label: "当前平台安全隔离级别：基础（不含系统调用过滤）",
			Capabilities: []string{
				"独立 PID（Setpgid）——OS 强制（syscall.Setpgid）",
				"文件权限 0700——OS 强制（os.Chmod）",
				"网络默认禁止——代码约定（无 seccomp/pfctl 不可靠）。v0.3.0 App Sandbox",
				"禁止 fork/execve——代码约定（无 seccomp）。v0.3.0 App Sandbox",
				"禁止 setuid/ptrace——代码约定（无 seccomp）。v0.3.0 App Sandbox",
			},
		}
	}
	return SecurityLevel{
		Platform: "linux",
		Level:    "L3",
		Label:    "当前平台安全隔离级别：完整（seccomp + namespace + 文件权限）",
		Capabilities: []string{
			"seccomp BPF 系统调用过滤",
			"CLONE_NEWNET 网络隔离",
			"文件权限 0700",
			"禁止 fork/execve/setuid/ptrace",
		},
	}
}
