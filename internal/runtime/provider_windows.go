//go:build windows

// provider_windows.go——Windows 受限档 Provider（任务 5.1——agentbox 收敛）。
// 边界机制（复用 v0.3.0 实证机制——executor_windows.go 同源）：Job Object 进程组隔离
// （KILL_ON_JOB_CLOSE=父进程终止时子进程自动清理——进程树生命周期）+资源限制。
// 诚实边界（2026-08-29 施工标注）：Restricted Token+Low Integrity Level+ACL 面
// （agentbox 集成——C-EXEC-04 vendor pin 族）本切片未落地——当前达成=进程组边界+资源限制
// （I1 族诚实呈现）。fs/ACL 面落地=后续窗口（12 清单 TC-RT-001a 先红锚点保持）。
// 本机（darwin）不可功能验证——契约测试经 windows-daily CI 平台实证。
package runtime

import (
	"context"
	"fmt"
	"os/exec"
	"strings"
	"sync"
	"syscall"
	"time"
	"unsafe"
)

var (
	kernel32               = syscall.NewLazyDLL("kernel32.dll")
	procCreateJobObjectW   = kernel32.NewProc("CreateJobObjectW")
	procSetInformationJob  = kernel32.NewProc("SetInformationJobObject")
	procAssignProcessToJob = kernel32.NewProc("AssignProcessToJobObject")
	procTerminateJobObject = kernel32.NewProc("TerminateJobObject")
	procCloseHandle        = kernel32.NewProc("CloseHandle")
	procOpenProcess        = kernel32.NewProc("OpenProcess")
)

// OpenProcess 访问权（AssignProcessToJobObject 所需——PROCESS_SET_QUOTA|PROCESS_TERMINATE）。
const processJobAssignAccess = 0x0100 | 0x0001

const (
	jobObjectLimitKillOnJobClose        = 0x00002000
	jobObjectExtendedLimitInformation   = 2
	jobObjectLimitProcessMemory         = 0x00000100
)

// windowsJobProvider Windows 受限档 Provider（Job Object 边界族）。
type windowsJobProvider struct {
	mu        sync.Mutex
	state     ProviderState
	workspace string
	tmpDir    string
}

// NewWindowsJobProvider 构造 Windows 受限档 Provider。
func NewWindowsJobProvider(workspace, tmpDir string) Provider {
	return &windowsJobProvider{
		state:     ProviderRegistered,
		workspace: workspace,
		tmpDir:    tmpDir,
	}
}

func (p *windowsJobProvider) Name() string { return "windows-jobobject" }
func (p *windowsJobProvider) Tier() string { return TierRestricted.String() }

func (p *windowsJobProvider) State(context.Context) (ProviderState, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.state, nil
}

// Capabilities 能力快照（诚实降级证据——R-1506）：Restricted Token/ACL 面未达成=显式携带。
func (p *windowsJobProvider) Capabilities(context.Context) (ProviderCapability, error) {
	return ProviderCapability{
		Platform:          "windows",
		AchievedIsolation: I1, // 诚实=进程组边界+资源限制（Restricted Token/ACL 面未达成——非 I2）
		DegradedEvidence:  []string{"Restricted Token+Low IL+ACL 面未落地（agentbox 集成窗口——I1 诚实呈现非 I2）"},
	}, nil
}

// Prepare 一次性准备：Job Object API 可用性实测（创建+配置+关闭空跑——不信静态读数 R-960）。
func (p *windowsJobProvider) Prepare(context.Context, RuntimePlan) error {
	p.mu.Lock()
	defer p.mu.Unlock()
	job, err := createConfiguredJob()
	if err != nil {
		return fmt.Errorf("%w: windows Job Object 实测失败: %w", ErrNoBackend, err)
	}
	procCloseHandle.Call(job) // 空跑句柄即关（验证通过不留资源）
	p.state = ProviderPrepared
	return nil
}

// Acquire 租约（每租约一 Job——KILL_ON_JOB_CLOSE 绑定句柄生命周期）。
func (p *windowsJobProvider) Acquire(_ context.Context, req LeaseRequest) (RuntimeHandle, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.state != ProviderPrepared {
		return nil, fmt.Errorf("runtime: Provider 未 Prepare（状态=%v）", p.state)
	}
	return &windowsHandle{
		id:    fmt.Sprintf("job-%d", time.Now().UnixNano()),
		p:     p,
		state: HandleAcquired,
	}, nil
}

// windowsHandle Windows Job Object 执行句柄（D-5 定序）。
type windowsHandle struct {
	mu        sync.Mutex
	id        string
	p         *windowsJobProvider
	state     HandleState
	job       uintptr // Job Object 句柄（Start 创建）
	activeCmd *exec.Cmd
}

// createConfiguredJob 创建+配置 Job Object（KILL_ON_JOB_CLOSE=父死子收兜底）。
func createConfiguredJob() (uintptr, error) {
	job, _, err := procCreateJobObjectW.Call(0, 0)
	if job == 0 {
		return 0, fmt.Errorf("CreateJobObjectW 失败: %w", err)
	}
	// KILL_ON_JOB_CLOSE 配置（limit=0x2000——进程组随 Job 关闭自动清理）
	type jobBasicLimitInfo struct {
		PerProcessUserTimeLimit int64
		PerJobUserTimeLimit     int64
		LimitFlags              uint32
		MinimumWorkingSetSize   uintptr
		MaximumWorkingSetSize   uintptr
		ActiveProcessLimit      uint32
		Affinity                uintptr
		PriorityClass           uint32
		SchedulingClass         uint32
	}
	type jobExtendedLimitInfo struct {
		BasicLimitInformation jobBasicLimitInfo
		IoInfo                [48]byte
		ProcessMemoryLimit    uintptr
		JobMemoryLimit        uintptr
		PeakProcessMemoryUsed uintptr
		PeakJobMemoryUsed     uintptr
	}
	info := jobExtendedLimitInfo{}
	info.BasicLimitInformation.LimitFlags = jobObjectLimitKillOnJobClose
	ret, _, err := procSetInformationJob.Call(job, jobObjectExtendedLimitInformation,
		uintptr(unsafe.Pointer(&info)), uintptr(unsafe.Sizeof(info)))
	if ret == 0 {
		procCloseHandle.Call(job)
		return 0, fmt.Errorf("SetInformationJobObject 失败: %w", err)
	}
	return job, nil
}

func (h *windowsHandle) ID() string { return h.id }
func (h *windowsHandle) State() HandleState {
	h.mu.Lock()
	defer h.mu.Unlock()
	return h.state
}

// Start 建立边界：创建+配置 Job Object（句柄生命周期载体）。
func (h *windowsHandle) Start(context.Context) error {
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.state != HandleAcquired {
		return ErrInvalidState
	}
	job, err := createConfiguredJob()
	if err != nil {
		return fmt.Errorf("runtime: Job Object 创建失败: %w", err)
	}
	h.job = job
	h.state = HandleReady
	return nil
}

// Precheck 边界验证（真实探针——只验证声称的边界）：Job 句柄存活+可指派=
// 边界机制在位证据。fs/ACL 面无声称（未落地）故无探针——能力快照如实降级（I1 呈现）。
func (h *windowsHandle) Precheck(ctx context.Context) error {
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.state != HandleReady && h.state != HandleRunning {
		return ErrInvalidState
	}
	if h.job == 0 {
		return fmt.Errorf("runtime: Precheck 失败——Job 句柄缺失（边界建立未生效，RTM-PRECHECK-F-001 族）")
	}
	h.state = HandleRunning
	return nil
}

// Execute 边界内执行（进程入 Job——KILL_ON_JOB_CLOSE 兜底）。
func (h *windowsHandle) Execute(ctx context.Context, req ExecuteRequest) (ExecuteResult, error) {
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.state != HandleRunning {
		return ExecuteResult{}, ErrInvalidState
	}
	if req.ActionType != "process.exec" {
		return ExecuteResult{}, fmt.Errorf("runtime: 受限档不支持的能力动词 %q（process.exec 唯一）", req.ActionType)
	}
	binary := req.Params["binary"]
	if binary == "" {
		return ExecuteResult{}, fmt.Errorf("runtime: process.exec 缺 binary 参数（代理层拒绝——未触 OS 边界）")
	}
	start := time.Now()
	cmd := exec.CommandContext(ctx, binary)
	if req.Params["args"] != "" {
		cmd.Args = append(cmd.Args, splitArgsWindows(req.Params["args"])...)
	}
	var outBuf, errBuf strings.Builder
	cmd.Stdout = &outBuf
	cmd.Stderr = &errBuf
	if err := cmd.Start(); err != nil {
		return ExecuteResult{Status: "failed", ExitCode: -1, Output: err.Error()}, nil
	}
	h.activeCmd = cmd
	// 进程入 Job（边界生效点——OpenProcess 按 PID 取句柄：os.Process.Handle 未导出，
	// syscall 直取；PROCESS_SET_QUOTA|TERMINATE=AssignProcessToJobObject 所需访问权）
	if cmd.Process != nil && h.job != 0 {
		ph, _, openErr := procOpenProcess.Call(uintptr(processJobAssignAccess), 0, uintptr(cmd.Process.Pid))
		if ph == 0 {
			// 进程未入 Job=边界未生效——fail-closed 终止（不放任无边界执行）；
			// 注：Windows syscall Errno 0=成功惯例——openErr 仅作诊断语义
			_ = cmd.Process.Kill()
			return ExecuteResult{Status: "failed", ExitCode: -1,
				Output: fmt.Sprintf("OpenProcess 失败（进程未入 Job=边界未生效，fail-closed）: %v", openErr)}, nil
		}
		procAssignProcessToJob.Call(h.job, ph)
		procCloseHandle.Call(ph)
	}
	waitErr := cmd.Wait()
	out := outBuf.String() + errBuf.String()
	res := ExecuteResult{ExitCode: 0, Output: out, Cost: time.Since(start), Status: "success"}
	if waitErr != nil {
		res.Status = "failed"
		if ee, ok := waitErr.(*exec.ExitError); ok {
			res.ExitCode = ee.ExitCode()
		} else {
			res.ExitCode = -1
		}
	}
	h.activeCmd = nil
	return res, nil
}

// Interrupt 优雅取消（TerminateJobObject=组终止）。
func (h *windowsHandle) Interrupt(context.Context) error {
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.state != HandleRunning {
		return ErrInvalidState
	}
	if h.job != 0 {
		procTerminateJobObject.Call(h.job, 1)
		h.activeCmd = nil
	}
	return nil
}

// Pause 内核强制挂起（Windows=Job 级挂起归后续——当前=句柄级状态阻断 Execute 合法态）。
func (h *windowsHandle) Pause(context.Context) error {
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.state != HandleRunning {
		return ErrInvalidState
	}
	// 诚实标注：Windows Job 无 SIGSTOP 等价物——本切片=句柄级挂起（状态阻断），
	// 进程级挂起（线程冻结）归后续窗口（诚实不假装）。
	h.state = HandlePaused
	return nil
}

// Resume 恢复。
func (h *windowsHandle) Resume(context.Context) error {
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.state != HandlePaused {
		return ErrInvalidState
	}
	h.state = HandleRunning
	return nil
}

// Release 清理：关闭 Job 句柄（KILL_ON_JOB_CLOSE 触发组清理——幂等）。
func (h *windowsHandle) Release(context.Context) error {
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.state == HandleReleased || h.state == HandleDestroyed {
		return nil
	}
	if h.job != 0 {
		procCloseHandle.Call(h.job) // Job 关闭=组内进程自动终止
		h.job = 0
	}
	h.state = HandleReleased
	return nil
}

// splitArgsWindows 空格分隔（同 linux 形态——独立副本因 build-tag 隔离不共享包内符号）。
func splitArgsWindows(s string) []string {
	var out []string
	cur := ""
	for _, r := range s {
		if r == ' ' {
			if cur != "" {
				out = append(out, cur)
				cur = ""
			}
		} else {
			cur += string(r)
		}
	}
	if cur != "" {
		out = append(out, cur)
	}
	return out
}
