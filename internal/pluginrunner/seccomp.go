// Package pluginrunner — seccomp 系统调用过滤配置。
// W3: 配置定义。W4: golang.org/x/sys/unix 实际加载。
// 设计依据：08 沙箱规范 §4、R137。
package pluginrunner

// SeccompProfile 定义 seccomp 过滤规则。
type SeccompProfile struct {
	DefaultAction   string   `json:"default_action"`   // "kill" | "errno"
	AllowedSyscalls []string `json:"allowed_syscalls"` // 白名单
}

// DefaultSeccomp 返回默认 seccomp 配置（deny-all + 最小白名单）。
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
