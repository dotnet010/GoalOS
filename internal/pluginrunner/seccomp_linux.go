//go:build linux

package pluginrunner

import (
	"fmt"
	"syscall"
	"unsafe"
)

// syscallNameToNumber 将常见系统调用名称映射到 Linux x86_64 编号。
var syscallNameToNumber = map[string]uint32{
	"read": 0, "write": 1, "close": 3, "stat": 4, "fstat": 5,
	"lseek": 8, "mmap": 9, "mprotect": 10, "munmap": 11, "brk": 12,
	"rt_sigprocmask": 14, "access": 21, "madvise": 28, "getpid": 39,
	"clone": 56, "exit": 60, "exit_group": 231, "fcntl": 72,
	"fsync": 74, "fdatasync": 75, "ftruncate": 77,
	"getdents": 78, "getdents64": 217, "getcwd": 79,
	"chdir": 80, "mkdir": 83, "rmdir": 84, "unlink": 87,
	"rename": 82, "openat": 257, "readlink": 89,
	"fstatfs": 138, "statfs": 137,
	"clock_gettime": 228, "gettimeofday": 96, "nanosleep": 35,
	"futex": 202, "gettid": 186, "sigaltstack": 131,
	"set_robust_list": 273, "prctl": 157, "arch_prctl": 158,
	"getrandom": 318, "rseq": 334,
}

const (
	seccompRetKill  = 0x00000000
	seccompRetAllow = 0x7FFF0000
	seccompRetErrno = 0x00050000
	auditArchX8664  = 0xC000003E
)

// ApplySeccomp 在当前进程中加载 seccomp BPF 过滤程序。
// 在子进程 exec 之前调用（fork 后、新程序执行前）。
func ApplySeccomp(profile *SeccompProfile) error {
	insns := buildSeccompBPF(profile)

	prog := &seccompProg{
		Len:    uint16(len(insns)),
		insns:  insns,
	}

	// PR_SET_NO_NEW_PRIVS = 36，必须在加载 seccomp 之前设置
	const prSetNoNewPrivs = 36
	if _, _, errno := syscall.RawSyscall(syscall.SYS_PRCTL, prSetNoNewPrivs, 1, 0); errno != 0 {
		return fmt.Errorf("seccomp: prctl(PR_SET_NO_NEW_PRIVS) 失败: %v", errno)
	}

	// PR_SET_SECCOMP = 22, SECCOMP_MODE_FILTER = 2
	const (
		prSetSeccomp      = 22
		seccompModeFilter = 2
	)

	if _, _, errno := syscall.RawSyscall(syscall.SYS_PRCTL, prSetSeccomp, seccompModeFilter, uintptr(unsafe.Pointer(prog))); errno != 0 {
		return fmt.Errorf("seccomp: prctl(PR_SET_SECCOMP) 失败: %v", errno)
	}

	return nil
}

// seccompProg 是 seccomp BPF 程序的包装。
type seccompProg struct {
	Len   uint16
	insns []seccompInstr
}

// seccompInstr 是 seccomp BPF 指令。
type seccompInstr struct {
	Code uint16
	Jt   uint8
	Jf   uint8
	K    uint32
}

// buildSeccompBPF 生成 deny-all + 白名单 BPF 程序。
// 格式: ld arch; jne kill; ld nr; { jeq allowed_i, jt=allow; } kill: ret KILL; allow: ret ALLOW。
func buildSeccompBPF(profile *SeccompProfile) []seccompInstr {
	killAct := uint32(seccompRetKill)
	if profile.DefaultAction == "errno" {
		killAct = seccompRetErrno | 1 // EPERM
	}

	insns := []seccompInstr{
		{Code: 0x20, K: 4},                                     // ld [4] — 加载 arch
		{Code: 0x15, Jt: 1, Jf: 0, K: auditArchX8664},         // jeq x86_64: Jt=1 跳过 KILL 继续; Jf=0 落入 KILL
		{Code: 0x06, K: killAct},                               // KILL — 错误架构
		{Code: 0x20, K: 0},                                     // ld [0] — 加载 syscall number
	}

	// 为每个允许的 syscall 添加 jeq 检查。Jt=N-i 确保命中时跳过剩余 jeq 到达 ALLOW。
	allowedNums := make([]uint32, 0)
	for _, name := range profile.AllowedSyscalls {
		if sysNo, ok := syscallNameToNumber[name]; ok {
			allowedNums = append(allowedNums, sysNo)
		}
	}
	N := len(allowedNums)
	for i, sysNo := range allowedNums {
		jt := uint8(N - i) // 跳过剩余 (N-1-i) 个 jeq + 1 个 KILL = N-i
		insns = append(insns, seccompInstr{Code: 0x15, Jt: jt, Jf: 0, K: sysNo})
	}

	// KILL (syscall not in whitelist)
	insns = append(insns, seccompInstr{Code: 0x06, K: killAct})
	// ALLOW
	insns = append(insns, seccompInstr{Code: 0x06, K: seccompRetAllow})

	return insns
}
