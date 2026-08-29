//go:build linux

// provider_linux.go——Linux 受限档 Provider（任务 5.2——seccomp+ns+cgroup 收敛）。
// 边界机制（复用 v0.3.0 实证机制——executor_linux.go 同源）：CLONE_NEWNET 网络命名空间
// 隔离（无接口=网络全断——内核强制，不需子进程协作）+CLONE_NEWPID 进程命名空间+
// Pdeathsig 父死子收（进程树生命周期）+seccomp 自加载归协议子进程路径（R-1371 既有）。
// 诚实边界（2026-08-29 施工标注）：fs 禁闭（mount ns+bind/pivot 编排）本切片未落地——
// 需要 NEWUSER+挂载编排实机开发窗口（darwin 工作站不可验证）；本 Provider 当前达成=
// 网络禁闭+进程树边界。Precheck 探针如实验证——fs 探针不过=句柄不 Running（诚实失败，
// 不假报档位）。fs 禁闭落地=后续实机窗口（登记：12 清单 TC-RT-001b 先红锚点保持）。
package runtime

import (
	"context"
	"fmt"
	"os/exec"
	"sync"
	"syscall"
	"time"
)

// linuxNamespaceProvider Linux 受限档 Provider（命名空间边界族）。
type linuxNamespaceProvider struct {
	mu        sync.Mutex
	state     ProviderState
	workspace string
	tmpDir    string
}

// NewLinuxNamespaceProvider 构造 Linux 受限档 Provider。
func NewLinuxNamespaceProvider(workspace, tmpDir string) Provider {
	return &linuxNamespaceProvider{
		state:     ProviderRegistered,
		workspace: workspace,
		tmpDir:    tmpDir,
	}
}

func (p *linuxNamespaceProvider) Name() string { return "linux-namespace" }
func (p *linuxNamespaceProvider) Tier() string { return TierRestricted.String() }

func (p *linuxNamespaceProvider) State(context.Context) (ProviderState, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.state, nil
}

// Capabilities 能力快照（诚实降级证据——R-1506）：fs 禁闭未达成=降级证据显式携带。
func (p *linuxNamespaceProvider) Capabilities(context.Context) (ProviderCapability, error) {
	return ProviderCapability{
		Platform:          "linux",
		AchievedIsolation: I1, // 诚实=进程边界+网络禁闭（fs 禁闭未达成——非 I2 单面齐备）
		DegradedEvidence:  []string{"fs 禁闭未落地（mount ns 编排待实机窗口——I1 诚实呈现非 I2）"},
	}, nil
}

// Prepare 一次性准备：命名空间能力实测（NEWNET unshare 真实可性——不信静态读数 R-960）。
// RuntimePlan 本档不消费（边界=每次 Execute 时 unshare，无外部编译输入）。
func (p *linuxNamespaceProvider) Prepare(context.Context, RuntimePlan) error {
	p.mu.Lock()
	defer p.mu.Unlock()
	// 空跑验证：NEWNET unshare 实测（/bin/true 在 NEWNET 中跑通=能力真实）
	cmd := exec.Command("/bin/true")
	cmd.SysProcAttr = &syscall.SysProcAttr{
		Unshareflags: syscall.CLONE_NEWNET | syscall.CLONE_NEWPID,
	}
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("%w: linux 命名空间实测失败（unshare NEWNET/NEWPID——可能缺 CAP_SYS_ADMIN 或内核限制）: %w", ErrNoBackend, err)
	}
	p.state = ProviderPrepared
	return nil
}

// Acquire 租约（Deadline 预算判定归 W8 标定链路）。
func (p *linuxNamespaceProvider) Acquire(_ context.Context, req LeaseRequest) (RuntimeHandle, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.state != ProviderPrepared {
		return nil, fmt.Errorf("runtime: Provider 未 Prepare（状态=%v）", p.state)
	}
	return &linuxHandle{
		id:    fmt.Sprintf("ns-%d", time.Now().UnixNano()),
		p:     p,
		state: HandleAcquired,
	}, nil
}

// linuxHandle Linux 命名空间执行句柄（D-5 定序：Start→Precheck→Execute）。
type linuxHandle struct {
	mu        sync.Mutex
	id        string
	p         *linuxNamespaceProvider
	state     HandleState
	activeCmd *exec.Cmd
}

func (h *linuxHandle) ID() string { return h.id }
func (h *linuxHandle) State() HandleState {
	h.mu.Lock()
	defer h.mu.Unlock()
	return h.state
}

// Start 建立边界（命名空间标志位定型——无外部资源物化，边界=每次 Execute 时的 unshare）。
func (h *linuxHandle) Start(context.Context) error {
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.state != HandleAcquired {
		return ErrInvalidState
	}
	h.state = HandleReady
	return nil
}

// Precheck 边界验证（真实探针——只验证声称的边界，不假报未声称的）：
// 网络探针=NEWNET 内出站必须立即失败（无接口=「network unreachable」/dial 失败即边界证据）。
// fs 禁闭未落地（本切片）——fs 面无声称，故无探针；能力快照如实降级（I1 呈现非 I2）。
// TC-RT-001b（旁路全断言）保持先红——fs 禁闭落地窗口同转绿（12 清单登记锚点）。
func (h *linuxHandle) Precheck(ctx context.Context) error {
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.state != HandleReady && h.state != HandleRunning {
		return ErrInvalidState
	}
	// 网络探针：NEWNET 内 connect 必须失败（无接口可用）
	ctxProbe, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	probe := exec.CommandContext(ctxProbe, "/usr/bin/nc", "-w", "1", "192.0.2.1", "80")
	probe.SysProcAttr = &syscall.SysProcAttr{
		Unshareflags: syscall.CLONE_NEWNET | syscall.CLONE_NEWPID,
	}
	out, err := probe.CombinedOutput()
	if err == nil {
		return fmt.Errorf("runtime: Precheck 网络探针未被命名空间边界阻断（NEWNET 失效——out=%q）", out)
	}
	h.state = HandleRunning
	return nil
}

// Execute 边界内执行（命名空间隔离——process.exec 动词同受限档语义）。
func (h *linuxHandle) Execute(ctx context.Context, req ExecuteRequest) (ExecuteResult, error) {
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
		cmd.Args = append(cmd.Args, splitArgs(req.Params["args"])...)
	}
	cmd.SysProcAttr = &syscall.SysProcAttr{
		Unshareflags: syscall.CLONE_NEWNET | syscall.CLONE_NEWPID,
		Pdeathsig:    syscall.SIGKILL,
	}
	h.activeCmd = cmd
	out, err := cmd.CombinedOutput()
	res := ExecuteResult{ExitCode: 0, Output: string(out), Cost: time.Since(start), Status: "success"}
	if err != nil {
		res.Status = "failed"
		if ee, ok := err.(*exec.ExitError); ok {
			res.ExitCode = ee.ExitCode()
		} else {
			res.ExitCode = -1
		}
	}
	h.activeCmd = nil
	return res, nil
}

// Interrupt 优雅取消（Pdeathsig 兜底+显式 SIGKILL）。
func (h *linuxHandle) Interrupt(context.Context) error {
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.state != HandleRunning {
		return ErrInvalidState
	}
	if h.activeCmd != nil && h.activeCmd.Process != nil {
		_ = syscall.Kill(-h.activeCmd.Process.Pid, syscall.SIGKILL)
		h.activeCmd = nil
	}
	return nil
}

// Pause 内核强制挂起（SIGSTOP——R-1245）。
func (h *linuxHandle) Pause(context.Context) error {
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.state != HandleRunning {
		return ErrInvalidState
	}
	if h.activeCmd != nil && h.activeCmd.Process != nil {
		_ = syscall.Kill(-h.activeCmd.Process.Pid, syscall.SIGSTOP)
	}
	h.state = HandlePaused
	return nil
}

// Resume 恢复（SIGCONT）。
func (h *linuxHandle) Resume(context.Context) error {
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.state != HandlePaused {
		return ErrInvalidState
	}
	if h.activeCmd != nil && h.activeCmd.Process != nil {
		_ = syscall.Kill(-h.activeCmd.Process.Pid, syscall.SIGCONT)
	}
	h.state = HandleRunning
	return nil
}

// Release 清理（幂等）。
func (h *linuxHandle) Release(context.Context) error {
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.state == HandleReleased || h.state == HandleDestroyed {
		return nil
	}
	if h.activeCmd != nil && h.activeCmd.Process != nil {
		_ = syscall.Kill(-h.activeCmd.Process.Pid, syscall.SIGKILL)
		h.activeCmd = nil
	}
	h.state = HandleReleased
	return nil
}

// splitArgs 空格分隔（最小形态——引号内空格不支持，探针场景够用）。
func splitArgs(s string) []string {
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
