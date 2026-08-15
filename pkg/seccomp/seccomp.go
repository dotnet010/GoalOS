// Package seccomp 提供系统调用过滤配置（v0.1.0 OS 级安全边界）。
// PluginRunner（父进程）使用此包生成 profile 并通过 InitMessage 传递给子进程。
// 插件二进制（子进程）使用此包在 init 阶段自加载 seccomp。
//
// Linux: prctl(PR_SET_SECCOMP) + BPF。macOS: no-op（通过进程边界 + 文件权限降级，R345）。
//
// 设计依据：08 沙箱规范 §4、R137、R345、会议 #63（Linus 方案）。
package seccomp

// Profile 定义 seccomp 过滤规则。
type Profile struct {
	DefaultAction   string   `json:"default_action"`   // "kill" | "errno"
	AllowedSyscalls []string `json:"allowed_syscalls"` // 白名单
}

// Default 返回 L0-L2 严格 seccomp 配置（deny-all + ~30 syscall 白名单）。
func Default() *Profile {
	return &Profile{
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
func (p *Profile) CanaryScan(data []byte, depth int, budgetRemaining int) (matched bool, budgetExhausted bool, err error) {
	// 骨架：depth=0 原始字节哈希匹配锚点（预算耗尽后继续命中——R-1382 契约对象）。
	// 实现归任务 3.26/3.27（代理层+写层检测点）。
	if depth == 0 {
		// 原始字节哈希匹配（不消耗预算——R-1382：预算耗尽后原始哈希匹配继续命中）
		return false, false, nil // 骨架：无匹配（实现归 3.26/3.27）
	}
	if budgetRemaining <= 0 {
		// 预算耗尽=fail-closed（guard_budget_exhausted + SecurityIncident 去重）
		return false, true, nil
	}
	return false, false, nil // 骨架：深度解码降级（实现归 3.26/3.27）
}
