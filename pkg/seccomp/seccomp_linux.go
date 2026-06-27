//go:build linux

// Package seccomp — Linux BPF 实现（v0.1.0 会议 #63 Linus 方案）。
// 插件二进制在 init 阶段调用 Apply(profile) 自加载 seccomp。
package seccomp

import (
	"fmt"
	"syscall"
	"unsafe"
)

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
	"socket": 41, "connect": 42, "bind": 49, "sendto": 44, "recvfrom": 45,
	"setsockopt": 54, "getsockname": 51, "getpeername": 52, "listen": 50,
	"accept": 43, "accept4": 288, "clone3": 435, "prlimit64": 302,
	"pread64": 17, "pwrite64": 18, "readv": 19, "writev": 20,
	"splice": 275, "sendfile": 40, "copy_file_range": 326, "fallocate": 285,
	"mremap": 25, "mlock": 149, "munlock": 150,
	"rt_sigaction": 13, "rt_sigreturn": 15, "tgkill": 234, "tkill": 200,
	"clock_nanosleep": 230, "timer_create": 222, "timer_settime": 223,
	"flock": 73, "sched_getaffinity": 204, "sched_yield": 24,
}

const (
	seccompRetKill  = 0x00000000
	seccompRetAllow = 0x7FFF0000
	seccompRetErrno = 0x00050000
	auditArchX8664  = 0xC000003E
)

type seccompInstr struct {
	Code uint16
	Jt   uint8
	Jf   uint8
	K    uint32
}

type seccompProg struct {
	Len   uint16
	insns []seccompInstr
}

// Apply 在当前进程中加载 seccomp BPF 过滤程序（v0.1.0 Linus 方案）。
// 在子进程 init 阶段调用——fork 后、任何业务逻辑之前。
// PR_SET_NO_NEW_PRIVS 必须先于 seccomp 设置。
func Apply(profile *Profile) error {
	insns := buildBPF(profile)
	prog := &seccompProg{Len: uint16(len(insns)), insns: insns}

	const prSetNoNewPrivs = 36
	if _, _, errno := syscall.RawSyscall(syscall.SYS_PRCTL, prSetNoNewPrivs, 1, 0); errno != 0 {
		return fmt.Errorf("seccomp: prctl(PR_SET_NO_NEW_PRIVS): %v", errno)
	}

	const prSetSeccomp = 22
	const seccompModeFilter = 2
	if _, _, errno := syscall.RawSyscall(syscall.SYS_PRCTL, prSetSeccomp, seccompModeFilter, uintptr(unsafe.Pointer(prog))); errno != 0 {
		return fmt.Errorf("seccomp: prctl(PR_SET_SECCOMP): %v", errno)
	}
	return nil
}

// buildBPF 生成 deny-all + 白名单 BPF 程序。
func buildBPF(profile *Profile) []seccompInstr {
	killAct := uint32(seccompRetKill)
	if profile.DefaultAction == "errno" {
		killAct = seccompRetErrno | 1
	}
	insns := []seccompInstr{
		{Code: 0x20, K: 4},
		{Code: 0x15, Jt: 1, Jf: 0, K: auditArchX8664},
		{Code: 0x06, K: killAct},
		{Code: 0x20, K: 0},
	}
	allowedNums := make([]uint32, 0)
	for _, name := range profile.AllowedSyscalls {
		if sysNo, ok := syscallNameToNumber[name]; ok {
			allowedNums = append(allowedNums, sysNo)
		}
	}
	N := len(allowedNums)
	for i, sysNo := range allowedNums {
		jt := uint8(N - i)
		insns = append(insns, seccompInstr{Code: 0x15, Jt: jt, Jf: 0, K: sysNo})
	}
	insns = append(insns, seccompInstr{Code: 0x06, K: killAct})
	insns = append(insns, seccompInstr{Code: 0x06, K: seccompRetAllow})
	return insns
}
