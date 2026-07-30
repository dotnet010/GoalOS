//go:build linux

// Package seccomp — Linux BPF 实现（v0.1.0 会议 #63 Linus 方案）。
// 插件二进制在 init 阶段调用 Apply(profile) 自加载 seccomp。
package seccomp

import (
	"fmt"
	"syscall"
	"unsafe"
)

// x86_64 syscall numbers
var syscallNameToNumberX8664 = map[string]uint32{
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

// ARM64 (aarch64) syscall numbers — v0.3.0 fix (C7)
var syscallNameToNumberARM64 = map[string]uint32{
	"read": 63, "write": 64, "close": 57, "stat": 0, "fstat": 80,
	"lseek": 62, "mmap": 222, "mprotect": 226, "munmap": 215, "brk": 214,
	"rt_sigprocmask": 135, "access": 0, "madvise": 233, "getpid": 172,
	"clone": 220, "exit": 93, "exit_group": 94, "fcntl": 25,
	"fsync": 82, "fdatasync": 83, "ftruncate": 46,
	"getdents": 0, "getdents64": 61, "getcwd": 17,
	"chdir": 49, "mkdir": 0, "rmdir": 0, "unlink": 0,
	"rename": 0, "openat": 56, "readlink": 0,
	"fstatfs": 44, "statfs": 43,
	"clock_gettime": 113, "gettimeofday": 169, "nanosleep": 101,
	"futex": 98, "gettid": 178, "sigaltstack": 132,
	"set_robust_list": 99, "prctl": 167, "arch_prctl": 0,
	"getrandom": 278, "rseq": 293,
	"socket": 198, "connect": 203, "bind": 200, "sendto": 206, "recvfrom": 207,
	"setsockopt": 208, "getsockname": 204, "getpeername": 205, "listen": 201,
	"accept": 202, "accept4": 242, "clone3": 435, "prlimit64": 261,
	"pread64": 67, "pwrite64": 68, "readv": 65, "writev": 66,
	"splice": 76, "sendfile": 71, "copy_file_range": 285, "fallocate": 47,
	"mremap": 216, "mlock": 228, "munlock": 229,
	"rt_sigaction": 134, "rt_sigreturn": 139, "tgkill": 131, "tkill": 130,
	"clock_nanosleep": 115, "timer_create": 107, "timer_settime": 110,
	"flock": 32, "sched_getaffinity": 123, "sched_yield": 124,
}

const (
	seccompRetKill  = 0x00000000
	seccompRetAllow = 0x7FFF0000
	seccompRetErrno = 0x00050000
	auditArchX8664  = 0xC000003E
	auditArchARM64  = 0xC00000B7 // v0.3.0: ARM64 support
)

// detectArch 通过 uname 检测 CPU 架构，返回对应的 audit arch 常量和 syscall 映射表。
// v0.3.0 fix (C7): 支持 ARM64——不再硬编码 x86_64 系统调用号。
func detectArch() (auditArch uint32, syscallMap map[string]uint32) {
	var utsname syscall.Utsname
	if err := syscall.Uname(&utsname); err == nil {
		// utsname.Machine 是 [65]int8，转为 string
		machine := make([]byte, 0, 65)
		for _, b := range utsname.Machine {
			if b == 0 {
				break
			}
			machine = append(machine, byte(b))
		}
		switch string(machine) {
		case "aarch64":
			return auditArchARM64, syscallNameToNumberARM64
		}
	}
	// 默认 x86_64
	return auditArchX8664, syscallNameToNumberX8664
}

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
// v0.3.0 fix (C7): 运行时检测 CPU 架构——ARM64 和 x86_64 双架构支持。
func buildBPF(profile *Profile) []seccompInstr {
	arch, syscallMap := detectArch()
	killAct := uint32(seccompRetKill)
	if profile.DefaultAction == "errno" {
		killAct = seccompRetErrno | 1
	}
	insns := []seccompInstr{
		{Code: 0x20, K: 4},                    // ld [4] — 加载 seccomp_data.arch
		{Code: 0x15, Jt: 1, Jf: 0, K: arch},   // jeq arch → 继续; 否则 kill
		{Code: 0x06, K: killAct},               // kill（架构不匹配）
		{Code: 0x20, K: 0},                     // ld [0] — 加载 seccomp_data.nr（系统调用号）
	}
	allowedNums := make([]uint32, 0)
	for _, name := range profile.AllowedSyscalls {
		if sysNo, ok := syscallMap[name]; ok && sysNo != 0 {
			allowedNums = append(allowedNums, sysNo)
		}
	}
	// 确保至少有一个允许的系统调用
	if len(allowedNums) == 0 {
		allowedNums = append(allowedNums, syscallMap["read"], syscallMap["write"], syscallMap["exit"], syscallMap["exit_group"])
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
