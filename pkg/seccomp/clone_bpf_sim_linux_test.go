//go:build linux

// clone_bpf_sim_linux_test.go — buildBPF 旗标级过滤的指令级模拟验证（R-1371）。
//
// 动机（CI 红出，2026-08-17）: 旗标级 BPF 逻辑仅在 Linux 真实执行——darwin 本地
// 永远跑不到。helper 子进程真实应用 seccomp 的形态在 CI 首次暴露两处缺陷
//（无旗标过滤→线程级 kill→sysmon 存活挂起 5m）。本测试以确定性 BPF 解释器
// 直接验证 buildBPF 产出的指令序列对夹具六向量的裁决——无需 CAP_SYS_ADMIN，
// 在任何 Linux 环境（含 CI runner）可执行，先于 helper 集成测试兜底。
//
// 纪律: 指令语义模拟（行为断言）——不依赖内核、不依赖权限。

package seccomp

import (
	"encoding/binary"
	"testing"
)

// simRun 对 buildBPF 产出的 BPF 程序做确定性解释执行（本程序使用的子集:
// LD[off]/JEQ/AND/RET——classic BPF 语义，pc 从 0 起）。
func simRun(t *testing.T, insns []seccompInstr, nr uint32, args0 uint64) uint32 {
	t.Helper()
	// seccomp_data: nr@0 arch@4 ip@8 args@16（args[0] 低 32@16 高 32@20）
	data := make([]byte, 64)
	binary.LittleEndian.PutUint32(data[0:4], nr)
	// arch 与程序自检常量一致（arch 检测是机器真相，本测试验证旗标逻辑）
	binary.LittleEndian.PutUint32(data[4:8], insns[1].K)
	binary.LittleEndian.PutUint64(data[16:24], args0)

	var a uint32
	for pc := 0; ; {
		if pc >= len(insns) {
			t.Fatalf("simRun: pc=%d 越界（程序未以 RET 结束）", pc)
		}
		in := insns[pc]
		switch in.Code {
		case 0x20: // LD W ABS
			off := int(in.K)
			if off+4 > len(data) {
				t.Fatalf("simRun: ld [%d] 越界", off)
			}
			a = binary.LittleEndian.Uint32(data[off : off+4])
			pc++
		case 0x15: // JEQ K
			if a == in.K {
				pc += 1 + int(in.Jt)
			} else {
				pc += 1 + int(in.Jf)
			}
		case 0x14: // ALU AND K
			a = a & in.K
			pc++
		case 0x06: // RET K
			return in.K
		default:
			t.Fatalf("simRun: 不支持指令 Code=0x%02x @pc=%d", in.Code, pc)
		}
	}
}

// TestCloneBPF_FixtureVectors — 夹具六向量+扩展向量的指令级裁决（R-1352/R-1371）。
// 断言: 放行向量→RET_ALLOW；拒绝向量（0/命名空间/高32）→RET_KILL_PROCESS；
// clone3→RET_ERRNO(ENOSYS)；非白名单 syscall→KILL_PROCESS（通用拒绝升级）。
func TestCloneBPF_FixtureVectors(t *testing.T) {
	cloneNr := detectArchSyscallNo(t, "clone")
	insns := buildBPF(Default())

	cases := []struct {
		name string
		nr   uint32
		args uint64
		want uint32
	}{
		// 夹具放行向量（R-1352 allowed_mask 子集）
		{"clone-vec-001 六旗标放行", cloneNr, 0x00050F00, seccompRetAllow},
		{"clone-vec-002 glibc pthread 放行", cloneNr, 0x003D0F00, seccompRetAllow},
		// 夹具拒绝向量
		{"clone-vec-003 flags=0 拒绝", cloneNr, 0x00000000, seccompRetKillProcess},
		{"clone-vec-004 CLONE_NEWNET 拒绝", cloneNr, 0x40000011, seccompRetKillProcess},
		{"clone-vec-005 CLONE_NEWUSER 拒绝", cloneNr, 0x10000000, seccompRetKillProcess},
		{"高 32 位非零拒绝", cloneNr, 0x1_00050F00, seccompRetKillProcess},
		{"CLONE_CHILD_SETTID 单独旗标（mask 内）", cloneNr, 0x01000000, seccompRetAllow},
		{"SIGCHLD 进程创建旗标（mask 外——fork 穿透口 K-03）", cloneNr, 0x00010000 | 0x11, seccompRetKillProcess},
		// clone3
		{"clone-vec-006 clone3 ENOSYS", clone3Nr, 0x40000000, seccompRetErrno | uint32(38)},
		// 通用拒绝升级（Default 白名单外 syscall——socket）
		{"socket 通用拒绝 KILL_PROCESS", detectArchSyscallNo(t, "socket"), 0, seccompRetKillProcess},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := simRun(t, insns, tc.nr, tc.args)
			if got != tc.want {
				t.Errorf("R-1371 模拟裁决: nr=%d args=0x%X → 0x%08X，期望 0x%08X", tc.nr, tc.args, got, tc.want)
			}
		})
	}
}

// TestCloneBPF_ErrnoProfile_KeepsErrno — DefaultAction=errno 的 profile 通用
// 拒绝保持 RET_ERRNO（不升级 KILL_PROCESS——errno 语义契约不变）。
func TestCloneBPF_ErrnoProfile_KeepsErrno(t *testing.T) {
	cloneNr := detectArchSyscallNo(t, "clone")
	insns := buildBPF(&Profile{DefaultAction: "errno", AllowedSyscalls: []string{"clone"}})
	if got := simRun(t, insns, cloneNr, 0x00050F00); got != seccompRetAllow {
		t.Errorf("errno profile 下 clone 六旗标应放行，实际 0x%08X", got)
	}
	if got := simRun(t, insns, cloneNr, 0x40000011); got != seccompRetKillProcess {
		t.Errorf("errno profile 下命名空间旗标仍应 KILL_PROCESS（R-1371 独立于默认动作），实际 0x%08X", got)
	}
	if got := simRun(t, insns, detectArchSyscallNo(t, "socket"), 0); got != seccompRetErrno|1 {
		t.Errorf("errno profile 通用拒绝应保持 RET_ERRNO|1，实际 0x%08X", got)
	}
}

// detectArchSyscallNo 返回当前架构下指定 syscall 名对应的编号（复用生产映射）。
func detectArchSyscallNo(t *testing.T, name string) uint32 {
	t.Helper()
	_, syscallMap := detectArch()
	n, ok := syscallMap[name]
	if !ok || n == 0 {
		t.Fatalf("前置: 架构映射缺 %q", name)
	}
	return n
}
