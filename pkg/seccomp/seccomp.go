// Package seccomp 提供系统调用过滤配置（v0.1.0 OS 级安全边界）。
// PluginRunner（父进程）使用此包生成 profile 并通过 InitMessage 传递给子进程。
// 插件二进制（子进程）使用此包在 init 阶段自加载 seccomp。
//
// Linux: prctl(PR_SET_SECCOMP) + BPF。macOS: no-op（通过进程边界 + 文件权限降级，R345）。
//
// 设计依据：08 沙箱规范 §4、R137、R345、会议 #63（Linus 方案）。
package seccomp

import "fmt"

// Profile 定义 seccomp 过滤规则。
type Profile struct {
	DefaultAction   string   `json:"default_action"`   // "kill" | "errno"
	AllowedSyscalls []string `json:"allowed_syscalls"` // 白名单
}

// Default 返回 L0-L2 严格 seccomp 配置（deny-all + ~30 syscall 白名单）。
// v0.3.0 fix（CI 红出）: 补 Go runtime 异步抢占三件套——tgkill（SIGURG 抢占信号）/
// rt_sigreturn（信号处理器返回）/sched_yield（runtime 让步路径）。插件二进制多为
// Go（shell-executor 即 Go 编译）——缺这三者时 Apply 后首个抢占事件=deny→
// KILL_PROCESS 击杀整个插件进程（CI 实测 SIGSYS 无输出）。
func Default() *Profile {
	return &Profile{
		DefaultAction: "kill",
		AllowedSyscalls: []string{
			"read", "write", "close", "exit", "exit_group", "getpid", "gettid",
			"mmap", "munmap", "mprotect", "brk", "madvise",
			"openat", "lseek", "fstat", "stat", "access", "getdents64",
			"clock_gettime", "gettimeofday", "nanosleep",
			"futex", "clone", "rt_sigprocmask", "sigaltstack",
			"tgkill", "rt_sigreturn", "sched_yield",
		},
	}
}

// Extended 返回 L3+ 扩展 seccomp 配置（deny-all + ~100 syscall 白名单）。
func Extended() *Profile {
	profile := Default()
	profile.AllowedSyscalls = append(profile.AllowedSyscalls,
		"socket", "connect", "bind", "sendto", "recvfrom", "setsockopt",
		"getsockname", "getpeername", "listen", "accept", "accept4",
		"clone3", "set_robust_list", "prlimit64",
		"pread64", "pwrite64", "readv", "writev", "splice", "sendfile",
		"copy_file_range", "ftruncate", "fallocate",
		"mremap", "mlock", "munlock",
		"rt_sigaction", "rt_sigreturn", "tgkill", "tkill",
		"clock_nanosleep", "timer_create", "timer_settime", "timer_gettime",
		"getcwd", "chdir", "unlink", "mkdir", "rmdir",
		"fcntl", "flock", "fsync", "fdatasync",
		"getrandom", "sched_getaffinity", "sched_yield",
	)
	return profile
}

// ForRiskLevel 根据风险等级返回对应的 seccomp 配置。
func ForRiskLevel(riskLevel string) *Profile {
	switch riskLevel {
	case "L3", "L4", "L5":
		return Extended()
	default:
		return Default()
	}
}

// CanaryScan — 金丝雀扫描入口（R-1341/R-1382——预算耗尽后原始字节哈希匹配继续命中）。
// 契约：depth=0 原始字节哈希匹配不降级（预算耗尽=fail-closed guard_budget_exhausted+
// SecurityIncident 去重）；depth>0 深度解码/规范化降级（预算内）。
// 骨架：扫描入口存在（行为载体）——实现归任务 3.26/3.27。
// ErrNotImplemented — 骨架阶段显式未实现错误（R-1455——诚实化：未实现≠合法负结果）。
// 下游只要检查 error，就不可能把骨架结果当作干净扫描结果使用。
var ErrNotImplemented = fmt.Errorf("seccomp: CanaryScan not implemented (skeleton stage)")

// CanaryScan — 金丝雀扫描入口（R-1341/R-1382——预算耗尽后原始字节哈希匹配继续命中）。
// 骨架诚实化（R-1455）：返回 ErrNotImplemented——fail-open 方向的桩=安全监控隐性失效，
// 显式 error 防止调用方误读为"扫描后确认无金丝雀"。实现归任务 3.26/3.27。
// SKELETON-LIMIT: 3.26/3.27 真实金丝雀检测归实现任务
func (p *Profile) CanaryScan(data []byte, depth int, budgetRemaining int) (matched bool, budgetExhausted bool, err error) {
	return false, false, ErrNotImplemented
}
