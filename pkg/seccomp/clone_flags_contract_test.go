//go:build linux

// clone_flags_contract_test.go — clone 旗标级 seccomp 过滤契约测试（R-1352/R-1371，LP-16）。
//
// 断言来源: test/fixtures/clone_flags_fixture.yaml（会议 #202/#203）：
//   - clone-vec-004: clone(CLONE_NEWNET=0x40000000) → 拒绝（userns/netns 逃逸面封死，K-01）
//   - clone-vec-006: clone3(nr 435) 任何旗标 → RET_ERRNO(ENOSYS)（BPF 无法读用户内存 flags）
//   - clone-vec-001: VM|FS|FILES|SIGHAND|THREAD|SYSVSEM(0x00050F00) → 放行（Go runtime 六旗标）
//
// 先红状态（当前为何红）: pkg/seccomp 的 buildBPF 仅按 syscall 号白名单放行
// clone/clone3，不检查 BPF args[0] 旗标——clone(CLONE_NEWNET) 与 clone3 均会
// 成功 → clone-vec-004 / clone-vec-006 两个拒绝契约必红。clone-vec-001 属放行
// 集，当前实现同样放行 → 测试当前绿（夹具语义：放行向量不适用先红，但断言体
// 保留——防未来过度过滤把线程创建一并拒掉）。
//
// 转绿任务: 计划任务 3.19/3.17（seccomp 旗标级 BPF 过滤：
//
//	放行条件 `(flags & ^ALLOWED_MASK)==0 && flags!=0 && 高 32 位==0`；
//	CLONE_NEW* 命名空间旗标 → SECCOMP_RET_KILL_PROCESS(0x80000000)——
//	注意 RET_KILL(线程级) 不够：Go runtime 的 sysmon 线程存活会导致进程挂起，
//	本测试会以超时形式红出该缺陷；clone3 → SECCOMP_RET_ERRNO(ENOSYS)）。
//	对应 R-1352/R-1371，验收夹具 = clone_flags_fixture.yaml 六向量全判。
//
// 隔离形态: 与 internal/statestore 的 reentry 契约测试同构——helper 场景
// （Apply 自加载 seccomp 后的裸 clone）以 exec.Command 重执行本测试二进制、
// 环境变量触发 helper 分支；helper 是纯子进程，被 seccomp 击杀不会拖死本测试包。
package seccomp

import (
	"context"
	"encoding/binary"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"runtime"
	"strings"
	"syscall"
	"testing"
	"time"
	"unsafe"
)

// cloneHelperEnv 标记子进程进入 helper 分支（TestMain re-exec 模式，同 statestore_reentry_contract_test.go）。
const cloneHelperEnv = "GOALOS_SECCOMP_CLONE_HELPER"

// seccompApplyEpermMarker — helper 在 Apply 因权限不足失败时的输出标记
// （GitHub runner 非 root：现代内核下 no_new_privs 不再豁免非特权进程安装
// seccomp 过滤器——需 CAP_SYS_ADMIN）。父测试据此经 sudo 重试（runner 免密
// sudo）；sudo 亦不可用则诚实 skip（契约前置缺失，非伪绿）。
const seccompApplyEpermMarker = "SECCOMP_APPLY_EPERM"

// Linux uapi/linux/sched.h clone 旗标（Go syscall 包未导出，夹具自声明）。
const (
	cloneFlagNewnet  = 0x40000000 // CLONE_NEWNET —— 网络命名空间（vec-004 拒绝）
	cloneFlagSigchld = 0x00000011 // SIGCHLD(17) —— clone 低字节退出信号
	cloneThreadFlags = 0x00050F00 // clone-vec-001: VM|FS|FILES|SIGHAND|THREAD|SYSVSEM
	// clone3Nr 由包常量提供（seccomp_linux.go——x86_64 与 arm64 均为 435）
)

// TestSandbox_CloneFlags_NamespaceRejected —— clone-vec-004（R-1371）。
// 断言来源: 夹具 clone-vec-004「CLONE_NEWNET 命名空间旗标——拒绝」。
// 先红: 当前 buildBPF 无旗标检查 → clone(CLONE_NEWNET) 成功 → helper 打印放行标记。
// 转绿: 3.19/3.17 旗标级 BPF 过滤 + SECCOMP_RET_KILL_PROCESS。
func TestSandbox_CloneFlags_NamespaceRejected(t *testing.T) {
	if os.Getenv(cloneHelperEnv) == "1" {
		runCloneNamespaceHelper()
		return
	}

	out, ws, err := runCloneHelper(t, "TestSandbox_CloneFlags_NamespaceRejected")

	// MUST 1: 当前实现放行 clone(CLONE_NEWNET)——helper 打印放行标记即违约。
	if strings.Contains(string(out), "clone_allowed=true") {
		t.Fatalf("R-1371 clone-vec-004 FAIL: clone(CLONE_NEWNET) 未被旗标级过滤拒绝（当前 buildBPF 仅 syscall 号白名单，无 args[0] 检查）。output:\n%s", out)
	}
	if err == nil {
		t.Errorf("R-1371 clone-vec-004 FAIL: helper 正常退出但未打印任何状态标记——拒绝未生效或场景未跑通。output:\n%s", out)
		return
	}
	if errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("R-1371 clone-vec-004 FAIL: helper 超时挂起——SECCOMP_RET_KILL(线程级) 下 Go runtime sysmon 线程存活致进程不死；转绿须用 SECCOMP_RET_KILL_PROCESS(0x80000000)。output:\n%s", out)
	}
	// sudo 包装下的信号转码形态：现代 sudo 重抛子进程致命信号（Signaled+SIGSYS）；
	// 旧版 sudo 以 128+信号号 退出码转码（Exited+159=128+SIGSYS）——两种形态均合法。
	sigOK := ws.Signaled() && ws.Signal() == syscall.SIGSYS
	exitOK := ws.Exited() && ws.ExitStatus() == 128+int(syscall.SIGSYS)
	if !sigOK && !exitOK {
		t.Errorf("R-1371 clone-vec-004 FAIL: helper 死亡形态非 SIGSYS（err=%v, status=%v）——seccomp 拒绝未生效。output:\n%s", err, ws, out)
	}
}

// TestSandbox_Clone3_Rejected —— clone-vec-006（R-1371）。
// 断言来源: 夹具 clone-vec-006「clone3(nr 435) 任何旗标——RET_ERRNO(ENOSYS)」。
// 先红: 当前 Extended() 白名单含 clone3 → 成功（或非 ENOSYS errno）→ 无 enosys=true。
// 转绿: 3.19/3.17 clone3 → SECCOMP_RET_ERRNO(ENOSYS)。
func TestSandbox_Clone3_Rejected(t *testing.T) {
	if os.Getenv(cloneHelperEnv) == "1" {
		runClone3Helper()
		return
	}

	out, _, err := runCloneHelper(t, "TestSandbox_Clone3_Rejected")

	// MUST 1: ENOSYS 是 errno 拒绝（非击杀）——helper 必须正常完成。
	if err != nil {
		t.Fatalf("R-1371 clone-vec-006 FAIL: helper 未正常退出（%v）——clone3 拒绝形态应为 RET_ERRNO(ENOSYS) 而非击杀。output:\n%s", err, out)
	}
	// MUST 2: 必须收到 ENOSYS。
	if !strings.Contains(string(out), "enosys=true") {
		t.Fatalf("R-1371 clone-vec-006 FAIL: clone3 未被拒绝为 ENOSYS（当前 Extended 白名单放行 clone3→成功）。output:\n%s", out)
	}
	// MUST 3: 明确无 enosys=false 记录。
	if strings.Contains(string(out), "enosys=false") {
		t.Errorf("R-1371 clone-vec-006 FAIL: helper 报告 enosys=false。output:\n%s", out)
	}
}

// TestSandbox_CloneFlags_ThreadAllowed —— clone-vec-001（R-1371）。
// 断言来源: 夹具 clone-vec-001「Go runtime 六旗标组合 VM|FS|FILES|SIGHAND|THREAD|SYSVSEM——放行」。
// 先红: 不适用——当前无旗标过滤，clone 本就放行 → 本测试当前绿属可接受
//
//	（夹具语义：放行向量）；断言体保留——未来旗标过滤若过度收紧（误拒线程
//	旗标向量）→ 本测试红出。
//
// 转绿: 3.19/3.17 旗标过滤实现必须保留 ALLOWED_MASK 内向量放行。
func TestSandbox_CloneFlags_ThreadAllowed(t *testing.T) {
	if os.Getenv(cloneHelperEnv) == "1" {
		runThreadCloneHelper()
		return
	}

	out, _, err := runCloneHelper(t, "TestSandbox_CloneFlags_ThreadAllowed")

	// MUST 1: 线程旗标向量不在拒绝集——helper 必须正常完成。
	if err != nil {
		t.Fatalf("R-1371 clone-vec-001 FAIL: helper 未正常退出（%v）——线程旗标向量不应被击杀。output:\n%s", err, out)
	}
	// MUST 2: 线程旗标向量必须放行。
	if !strings.Contains(string(out), "thread_created=true") {
		t.Fatalf("R-1371 clone-vec-001 FAIL: 线程旗标组合 0x%X 未放行——旗标过滤过度收紧（Go runtime 线程创建被拒）。output:\n%s", cloneThreadFlags, out)
	}
	// MUST 3: 无失败标记。
	if strings.Contains(string(out), "thread_created=false") {
		t.Errorf("R-1371 clone-vec-001 FAIL: helper 报告线程创建失败。output:\n%s", out)
	}
}

// ─── helper 分支 ─────────────────────────────────────────────────────────

// runCloneNamespaceHelper: Apply(Default) 后裸 clone(CLONE_NEWNET|SIGCHLD)。
// 转绿后：seccomp 旗标过滤在 syscall 边界击杀进程 → helper 不会打印任何标记。
func runCloneNamespaceHelper() {
	runtime.LockOSThread()
	runtime.GOMAXPROCS(1)
	if err := Apply(Default()); err != nil {
		if isPermDeniedErr(err) {
			fmt.Fprintf(os.Stderr, "%s: %v\n", seccompApplyEpermMarker, err)
			os.Exit(77)
		}
		fmt.Fprintf(os.Stderr, "CLONE_HELPER: Apply: %v\n", err)
		os.Exit(1)
	}
	tid, errno := rawCloneThread(cloneFlagNewnet | cloneFlagSigchld)
	if errno != 0 {
		fmt.Fprintf(os.Stderr, "CLONE_HELPER: clone(CLONE_NEWNET) errno=%d（环境无 CAP_SYS_ADMIN 无法演示内核路径）\n", errno)
		os.Exit(2)
	}
	fmt.Printf("CLONE_HELPER: clone_allowed=true tid=%d\n", tid)
	os.Exit(0)
}

// runClone3Helper: Apply(Extended) 后裸 clone3(nr 435) 任何旗标。
// 转绿后：clone3 → RET_ERRNO(ENOSYS) → helper 打印 enosys=true。
func runClone3Helper() {
	runtime.LockOSThread()
	runtime.GOMAXPROCS(1)
	if err := Apply(Extended()); err != nil {
		if isPermDeniedErr(err) {
			fmt.Fprintf(os.Stderr, "%s: %v\n", seccompApplyEpermMarker, err)
			os.Exit(77)
		}
		fmt.Fprintf(os.Stderr, "CLONE3_HELPER: Apply: %v\n", err)
		os.Exit(1)
	}
	// struct clone_args（kernel 5.3+）：flags 偏移 0（u64），exit_signal 偏移 32。
	var args [64]byte
	binary.LittleEndian.PutUint64(args[0:8], uint64(cloneFlagNewnet))
	binary.LittleEndian.PutUint64(args[32:40], uint64(syscall.SIGCHLD))
	r1, _, errno := syscall.RawSyscall(uintptr(clone3Nr), uintptr(unsafe.Pointer(&args)), uintptr(len(args)), 0)
	if r1 == 0 {
		// 子进程（无 CLONE_VM——私有内存拷贝）：立即裸退出，不触碰运行时状态。
		syscall.RawSyscall(syscall.SYS_EXIT, 0, 0, 0)
		os.Exit(9) // 不可达安全网
	}
	if errno == syscall.ENOSYS {
		fmt.Println("CLONE3_HELPER: enosys=true")
		os.Exit(0)
	}
	fmt.Printf("CLONE3_HELPER: enosys=false errno=%d pid=%d\n", errno, r1)
	os.Exit(0)
}

// runThreadCloneHelper: Apply(Default) 后裸 clone(VM|FS|FILES|SIGHAND|THREAD|SYSVSEM)。
// 子进程路径由 rawCloneThread 汇编直接 SYS_EXIT——共享内存/共享栈语义下安全。
func runThreadCloneHelper() {
	runtime.LockOSThread()
	runtime.GOMAXPROCS(1)
	if err := Apply(Default()); err != nil {
		if isPermDeniedErr(err) {
			fmt.Fprintf(os.Stderr, "%s: %v\n", seccompApplyEpermMarker, err)
			os.Exit(77)
		}
		fmt.Fprintf(os.Stderr, "THREAD_HELPER: Apply: %v\n", err)
		os.Exit(1)
	}
	tid, errno := rawCloneThread(cloneThreadFlags)
	if errno != 0 {
		fmt.Fprintf(os.Stderr, "THREAD_HELPER: thread_created=false errno=%d\n", errno)
		os.Exit(2)
	}
	fmt.Printf("THREAD_HELPER: thread_created=true tid=%d\n", tid)
	os.Exit(0)
}

// runCloneHelper 执行 helper 子进程；环境无 CAP_SYS_ADMIN（GitHub runner 非 root）
// 时经 sudo 重试（runner 免密 sudo）。两者皆不可用→诚实 skip（契约前置缺失）。
func runCloneHelper(t *testing.T, testName string) (string, syscall.WaitStatus, error) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	attempt := func(sudo bool) (string, syscall.WaitStatus, error) {
		var cmd *exec.Cmd
		if sudo {
			cmd = exec.CommandContext(ctx, "sudo", "env", cloneHelperEnv+"=1",
				os.Args[0], "-test.run=^"+testName+"$")
		} else {
			cmd = exec.CommandContext(ctx, os.Args[0], "-test.run=^"+testName+"$")
			cmd.Env = append(os.Environ(), cloneHelperEnv+"=1")
		}
		out, err := cmd.CombinedOutput()
		var ws syscall.WaitStatus
		if cmd.ProcessState != nil {
			if s, ok := cmd.ProcessState.Sys().(syscall.WaitStatus); ok {
				ws = s
			}
		}
		return string(out), ws, err
	}

	out, ws, err := attempt(false)
	if strings.Contains(out, seccompApplyEpermMarker) {
		if _, lookErr := exec.LookPath("sudo"); lookErr != nil {
			t.Skipf("环境无 CAP_SYS_ADMIN 且无 sudo——seccomp 过滤器无法安装，内核路径契约不可在此环境验证: %s", strings.TrimSpace(out))
		}
		sudoOut, sudoWS, sudoErr := attempt(true)
		if strings.Contains(sudoOut, seccompApplyEpermMarker) {
			t.Skipf("sudo 亦无法安装 seccomp 过滤器——环境不满足契约前置: %s", strings.TrimSpace(sudoOut))
		}
		out, ws, err = sudoOut, sudoWS, sudoErr
	}
	return out, ws, err
}

// isPermDeniedErr 判断 Apply 失败是否权限类（EPERM/EACCES——环境无 CAP_SYS_ADMIN）。
func isPermDeniedErr(err error) bool {
	if err == nil {
		return false
	}
	s := err.Error()
	return strings.Contains(s, "permission denied") || strings.Contains(s, "operation not permitted")
}
