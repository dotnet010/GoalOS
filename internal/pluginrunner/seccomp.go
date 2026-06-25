// Package pluginrunner — seccomp 系统调用过滤配置。
// Linux: executor_linux.go 中 sanitizeChildProcess 通过 PR_SET_NO_NEW_PRIVS +
//   CLONE_NEWNET 网络隔离 + seccomp BPF 提供三层防护。
// macOS: executor_darwin.go 中通过独立进程组隔离。
// 设计依据：08 沙箱规范 §4、R137、R345（分级白名单 v1.1.0）。
package pluginrunner

// SeccompProfile 定义 seccomp 过滤规则。
type SeccompProfile struct {
	DefaultAction   string   `json:"default_action"`   // "kill" | "errno"
	AllowedSyscalls []string `json:"allowed_syscalls"` // 白名单
}

// DefaultSeccomp 返回 L0-L2 严格 seccomp 配置（deny-all + ~30 syscall 白名单）。
// 适用于低风险 Action：fs.read, browser.open, fs.write。
func DefaultSeccomp() *SeccompProfile {
	return &SeccompProfile{
		DefaultAction: "kill",
		AllowedSyscalls: []string{
			"read", "write", "close", "exit", "exit_group", "getpid", "gettid",
			"mmap", "munmap", "mprotect", "brk", "madvise",
			"openat", "lseek", "fstat", "stat", "access", "getdents64",
			"clock_gettime", "gettimeofday", "nanosleep",
			"futex", "clone", "rt_sigprocmask", "sigaltstack",
		},
	}
}

// ExtendedSeccomp 返回 L3+ 扩展 seccomp 配置（deny-all + ~100 syscall 白名单）。
// 适用于高风险 Action：shell.execute, database.delete, payment.initiate。
// 允许网络 socket/connect/bind/sendto/recvfrom 和多线程 clone。
// 安全由 Governance 审批补偿——不是放宽安全，是风险分级管控。
func ExtendedSeccomp() *SeccompProfile {
	profile := DefaultSeccomp()
	profile.AllowedSyscalls = append(profile.AllowedSyscalls,
		// 网络
		"socket", "connect", "bind", "sendto", "recvfrom", "setsockopt",
		"getsockname", "getpeername", "listen", "accept", "accept4",
		// 多线程/进程
		"clone", "clone3", "set_robust_list", "prlimit64",
		// 高级 I/O
		"pread64", "pwrite64", "readv", "writev", "splice", "sendfile",
		"copy_file_range", "ftruncate", "fallocate",
		// 内存管理
		"mremap", "mlock", "munlock",
		// 信号
		"rt_sigaction", "rt_sigreturn", "tgkill", "tkill",
		// 时间
		"clock_nanosleep", "timer_create", "timer_settime", "timer_gettime",
		// 其他
		"getcwd", "chdir", "unlink", "mkdir", "rmdir",
		"fcntl", "flock", "fsync", "fdatasync",
		"getrandom", "sched_getaffinity", "sched_yield",
	)
	return profile
}

// SeccompForRiskLevel 根据风险等级返回对应的 seccomp 配置（v1.1.0 分级白名单）。
// L0-L2 → 严格白名单（~30 syscall）。L3/L4/L5 → 扩展白名单（~100 syscall）。
// 未知等级 → 严格白名单（安全默认：fail-secure）。
func SeccompForRiskLevel(riskLevel string) *SeccompProfile {
	switch riskLevel {
	case "L3", "L4", "L5":
		return ExtendedSeccomp()
	default: // L0, L1, L2, 或未知——安全默认走严格白名单
		return DefaultSeccomp()
	}
}
