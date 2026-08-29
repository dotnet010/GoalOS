//go:build darwin

// provider_darwin.go——任务 5.3：macOS Seatbelt 收敛为受限档（T1）Provider。
// 承诺语义精确化（R-1479/R-1532/R-1571）：Seatbelt=约定级纵深防御（sandbox-exec 自
// macOS 10.15 起为弃用 API——弃用 API 具名清单指针=06 §1.3 macOS 行「约定级」标注）。
// AchievedIsolation=I2（OS 原生强制单面——文件系统禁闭族+网络禁闭；非 syscall 过滤族）。
// 诚实边界：sandbox-exec 缺失=Prepare 失败（ErrNoBackend）——无静默降级
// （旧 executor 路径的 Setpgid 降级不进入 Provider 承诺面——R-1599 诚实降级纪律）。
package runtime

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/goalos/goalos/internal/sandbox"
)

// profile 单一来源=internal/sandbox.RestrictedSeatbeltProfile()（R-1641③——禁止副本）。

// darwinSeatbeltProvider 受限档 Provider（T1——Seatbelt 边界直接执行面）。
type darwinSeatbeltProvider struct {
	mu        sync.Mutex
	state     ProviderState
	workspace string // 工作区卷根（profile WORKSPACE_DIR 注入）
	tmpDir    string // 临时目录根（profile TMP_DIR 注入）
	homeDir   string
}

// NewDarwinSeatbeltProvider 构造受限档 Provider（workspace/tmpDir=边界允许的唯二写入面）。
// 路径全部 firmlink 规范化（EvalSymlinks）——SBPL 按真实路径匹配，/tmp≠/private/tmp
// （2026-08-29 实证：未规范化导致 workspace 写允许与 TARGET_BINARY 放行全部失配）。
func NewDarwinSeatbeltProvider(workspace, tmpDir string) Provider {
	home, _ := os.UserHomeDir()
	if c, err := filepath.EvalSymlinks(workspace); err == nil {
		workspace = c
	}
	if c, err := filepath.EvalSymlinks(tmpDir); err == nil {
		tmpDir = c
	}
	if c, err := filepath.EvalSymlinks(home); err == nil && c != "" {
		home = c
	}
	return &darwinSeatbeltProvider{
		state:     ProviderRegistered,
		workspace: workspace,
		tmpDir:    tmpDir,
		homeDir:   home,
	}
}

func (p *darwinSeatbeltProvider) Name() string { return "darwin-seatbelt" }
func (p *darwinSeatbeltProvider) Tier() string { return TierRestricted.String() }

func (p *darwinSeatbeltProvider) State(context.Context) (ProviderState, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.state, nil
}

// Capabilities 能力快照（降级证据显式化——R-1506；Seatbelt 无降级面：要么在要么不在）。
func (p *darwinSeatbeltProvider) Capabilities(context.Context) (ProviderCapability, error) {
	return ProviderCapability{
		Platform:          "darwin",
		AchievedIsolation: I2, // OS 原生强制单面（禁闭族——fs+net；非 syscall 过滤）
		WarmPool:          false,
	}, nil
}

// Prepare 一次性准备：sandbox-exec 可用性探测（不可用=ErrNoBackend 诚实失败，无降级）。
// RuntimePlan 本档不消费（profile=embed 单源，无外部编译输入）。
func (p *darwinSeatbeltProvider) Prepare(_ context.Context, _ RuntimePlan) error {
	p.mu.Lock()
	defer p.mu.Unlock()
	if _, err := exec.LookPath("sandbox-exec"); err != nil {
		return fmt.Errorf("%w: darwin sandbox-exec 不可用（Seatbelt 弃用 API 移除场景）", ErrNoBackend)
	}
	p.state = ProviderPrepared
	return nil
}

// Acquire 租约（冷启动——本版本无热池；Deadline 预算判定归 W8 标定链路）。
func (p *darwinSeatbeltProvider) Acquire(_ context.Context, req LeaseRequest) (RuntimeHandle, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.state != ProviderPrepared {
		return nil, fmt.Errorf("runtime: Provider 未 Prepare（状态=%v）", p.state)
	}
	// R-1643 裁决④：契约声明网络能力→profile 网络授权变体（data_sharing 审批已在治理上游完成——
	// 无契约/无网络能力=全拒变体，fail-closed）
	netCaps := false
	if req.Contract != nil {
		for _, c := range req.Contract.Claims().Capabilities {
			for _, prefix := range []string{"web.", "browser.", "net.", "http."} {
				if strings.HasPrefix(c, prefix) {
					netCaps = true
				}
			}
		}
	}
	return &seatbeltHandle{
		id:    fmt.Sprintf("sb-%d", time.Now().UnixNano()),
		p:     p,
		state: HandleAcquired,
		netCaps: netCaps,
	}, nil
}

// seatbeltHandle 受限档执行句柄（D-5 定序：Start 物化策略→Precheck 探针验证→Execute）。
type seatbeltHandle struct {
	mu          sync.Mutex
	id          string
	p           *darwinSeatbeltProvider
	state       HandleState
	profilePath string
	activeCmd   *exec.Cmd // 当前执行进程（Interrupt/Pause/Resume 对象）
	netCaps     bool      // 契约声明网络能力（R-1643 裁决④——profile 变体选择数据源）
}

func (h *seatbeltHandle) ID() string { return h.id }

func (h *seatbeltHandle) State() HandleState {
	h.mu.Lock()
	defer h.mu.Unlock()
	return h.state
}

// Start 建立边界：物化 Seatbelt profile（0600——策略文件非世界可读）。
func (h *seatbeltHandle) Start(context.Context) error {
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.state != HandleAcquired {
		return ErrInvalidState
	}
	if err := os.MkdirAll(h.p.tmpDir, 0700); err != nil {
		return fmt.Errorf("runtime: tmpDir 建立失败: %w", err)
	}
	// R-1643 裁决④：按契约网络能力选变体（授权=端口级放行 tcp 443/80；未授权=全拒）
	profile, err := sandbox.RestrictedSeatbeltProfileForNetwork(h.netCaps)
	if err != nil {
		return fmt.Errorf("runtime: profile 变体产出失败（单源漂移 fail-closed）: %w", err)
	}
	h.profilePath = filepath.Join(h.p.tmpDir, h.id+".sb")
	if err := os.WriteFile(h.profilePath, []byte(profile), 0600); err != nil {
		return fmt.Errorf("runtime: profile 物化失败: %w", err)
	}
	h.state = HandleReady
	return nil
}

// Precheck 边界验证（真实探针——非形式检查）：Option B 语义三探针——①写工作区外
// ②读敏感目录③出站连接，全部必须被子进程级 EPERM 拒绝（排除 execvp 级假象——
// 输出含 "sandbox-exec:" 前缀=边界从未生效的伪证，一律不计）。任一未拒绝=
// 边界建立但未生效→PrecheckFailed 语义（RTM-PRECHECK-F-001 族）。
func (h *seatbeltHandle) Precheck(ctx context.Context) error {
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.state != HandleReady && h.state != HandleRunning {
		return ErrInvalidState
	}
	home := h.p.homeDir
	probes := []struct{ name, binary, args string }{
		// Option B 语义三探针（会议 #256 语义裁决）：写禁闭/敏感目录禁读/网络禁闭
		{"fs-write-outside-workspace", "/usr/bin/touch", filepath.Join(home, ".goalos-precheck-probe")},
		{"sensitive-dir-read", "/bin/cat", filepath.Join(home, ".ssh")},
		{"net-outbound", "/usr/bin/nc", "-v -w 1 127.0.0.1 9"},
	}
	for _, probe := range probes {
		out, err := h.execInBoundary(probe.binary, strings.Fields(probe.args)...)
		// 反虚假绿：execvp 级失败（sandbox-exec: 前缀）不是边界证据——必须子进程级 EPERM
		denied := err != nil && strings.Contains(out, "Operation not permitted") &&
			!strings.Contains(out, "sandbox-exec:")
		if !denied {
			return fmt.Errorf("runtime: Precheck 探针 %s 未被 OS 边界拒绝（边界建立但未生效——out=%q err=%v）",
				probe.name, out, err)
		}
	}
	h.state = HandleRunning
	return nil
}

// Execute 边界内直接执行（capability 动词=process.exec；Params: binary+args 空格分隔）。
// 参数缺失=代理层拒绝（未触 OS——BypassCounters.RecordProxyRefusal 对照组数据源）。
func (h *seatbeltHandle) Execute(ctx context.Context, req ExecuteRequest) (ExecuteResult, error) {
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
	out, err := h.execInBoundary(binary, strings.Fields(req.Params["args"])...)
	res := ExecuteResult{
		ExitCode: 0,
		Output:   out,
		Cost:     time.Since(start),
		Status:   "success",
	}
	if err != nil {
		res.Status = "failed"
		var ee *exec.ExitError
		if errors.As(err, &ee) {
			res.ExitCode = ee.ExitCode()
		} else {
			res.ExitCode = -1
		}
	}
	return res, nil
}

// Interrupt 优雅取消（当前执行进程 SIGKILL——probe 短命语义；child 已退出=成功）。
func (h *seatbeltHandle) Interrupt(context.Context) error {
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.state != HandleRunning {
		return ErrInvalidState
	}
	if h.activeCmd != nil && h.activeCmd.Process != nil {
		_ = syscall.Kill(-h.activeCmd.Process.Pid, syscall.SIGKILL) // 进程组（Setpgid）
		h.activeCmd = nil
	}
	return nil
}

// Pause 内核强制挂起（R-1245 SIGSTOP——句柄级挂起：阻断 Execute 合法态）。
func (h *seatbeltHandle) Pause(context.Context) error {
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
func (h *seatbeltHandle) Resume(context.Context) error {
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

// Release 清理：杀活进程+删 profile 文件（幂等——二次调用成功）。
func (h *seatbeltHandle) Release(context.Context) error {
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.state == HandleReleased || h.state == HandleDestroyed {
		return nil
	}
	if h.activeCmd != nil && h.activeCmd.Process != nil {
		_ = syscall.Kill(-h.activeCmd.Process.Pid, syscall.SIGKILL)
		h.activeCmd = nil
	}
	if h.profilePath != "" {
		_ = os.Remove(h.profilePath)
	}
	h.state = HandleReleased
	return nil
}

// execInBoundary 在 Seatbelt 边界内执行命令（profile 物化路径+三参数注入）。
func (h *seatbeltHandle) execInBoundary(binary string, args ...string) (string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	// TARGET_BINARY 同样需 firmlink 规范化（SBPL 按真实路径匹配 literal）
	if c, err := filepath.EvalSymlinks(binary); err == nil {
		binary = c
	}
	cmd := exec.CommandContext(ctx, "/usr/bin/sandbox-exec",
		"-f", h.profilePath,
		"-D", "WORKSPACE_DIR="+h.p.workspace,
		"-D", "TMP_DIR="+h.p.tmpDir,
		"-D", "HOME_DIR="+h.p.homeDir,
		"-D", "TARGET_BINARY="+binary, // process-exec 仅放行目标本身（全 deny=execvp 自拒假象）
		"--", binary)
	cmd.Args = append(cmd.Args, args...)
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true} // 进程组（Interrupt/Pause 作用域）
	out, err := cmd.CombinedOutput()
	return string(out), err
}
