//go:build linux || windows

// provider_agentbox.go——agentbox 平台受限档 Provider（任务 5.1+5.2 收敛落地——
// C-EXEC-04 vendor pin 条件兑现：pin=v0.0.0-20260409110136-e003e7574798+vendor 目录锁定+
// 热点审查见开发日志 2026-08-29）。
// 单一治理权威纪律：agentbox 内建命令分类器=pass-through 透传（全 Sandboxed——
// GoalOS 治理五引擎是唯一裁决者，agentbox 分类面不二次治理——R-906a 治理不变量同构）。
// 边界机制=agentbox 平台族：Linux=namespace+Landlock+seccomp BPF；
// Windows=Restricted Token+Job Object+Low IL+ACLs——TC-RT-001a/b 转绿机制。
package runtime

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/zhangyunhao116/agentbox"
)

// passthroughClassifier GoalOS 治理唯一权威——agentbox 分类器透传（全 Sandboxed）。
// 禁止 agentbox 规则组二次裁决（R-906a：治理决策在 Governance 五引擎，执行机制不评估）。
type passthroughClassifier struct{}

func (passthroughClassifier) Classify(string) agentbox.ClassifyResult {
	return agentbox.ClassifyResult{Decision: agentbox.Sandboxed}
}

func (passthroughClassifier) ClassifyArgs(string, []string) agentbox.ClassifyResult {
	return agentbox.ClassifyResult{Decision: agentbox.Sandboxed}
}

// agentboxProvider agentbox 平台受限档 Provider（Linux/Windows 共用——平台机制归 agentbox）。
type agentboxProvider struct {
	mu        sync.Mutex
	state     ProviderState
	workspace string
	tmpDir    string
	platform  string // "linux"|"windows"
	mgr       agentbox.Manager
}

// NewAgentboxProvider 构造 agentbox 受限档 Provider（platform=GOOS 值——审计可读）。
func NewAgentboxProvider(workspace, tmpDir, platform string) Provider {
	return &agentboxProvider{
		state:     ProviderRegistered,
		workspace: workspace,
		tmpDir:    tmpDir,
		platform:  platform,
	}
}

func (p *agentboxProvider) Name() string { return p.platform + "-agentbox" }
func (p *agentboxProvider) Tier() string { return TierRestricted.String() }

func (p *agentboxProvider) State(context.Context) (ProviderState, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.state, nil
}

// Capabilities 能力快照（agentbox 平台机制真实达成——I2 单面齐备=fs 禁闭面成立）。
func (p *agentboxProvider) Capabilities(context.Context) (ProviderCapability, error) {
	return ProviderCapability{
		Platform:          p.platform,
		AchievedIsolation: I2, // agentbox=fs 禁闭面（Landlock/ACL）——单面成立（R-1506 阶梯）
		WarmPool:          false,
	}, nil
}

// Prepare 一次性准备：agentbox Manager 构造+平台可用性实测（Available——不信静态读数）。
func (p *agentboxProvider) Prepare(_ context.Context, _ RuntimePlan) error {
	p.mu.Lock()
	defer p.mu.Unlock()
	// WritableRoots 必须先存在——Windows ACL 授予（addAllowACE→GetNamedSecurityInfoW）
	// 对不存在路径直接失败，Exec 期才炸（2026-08-30 Windows 实机前置发现）。
	for _, root := range []string{p.workspace, p.tmpDir} {
		if err := os.MkdirAll(root, 0o755); err != nil {
			return fmt.Errorf("%w: WritableRoot 创建失败 %s: %w", ErrNoBackend, root, err)
		}
	}
	cfg := agentbox.DefaultConfig()
	cfg.Classifier = passthroughClassifier{} // 单一治理权威——透传
	cfg.Filesystem.WritableRoots = []string{p.workspace, p.tmpDir}
	// Windows DefaultConfig DenyWrite 含 home——工作区默认在用户目录下（生产接线
	// home\Goals / 测试 t.TempDir）时前缀冲突=Manager 构造失败=受限档整档不可用
	// （2026-08-30 Win11 实机实证，TC-RT-001a 先红）。裁决：剔除与 WritableRoots
	// 前缀冲突的 DenyWrite 条目（仅 home 会命中），系统目录 deny 保留；安全性不依赖
	// 被剔条目——Tier1=Low IL 写阻（home 对象=Medium IL 天然拒写），Tier2=沙箱用户
	// 对他人 profile 无 DACL 权限；DenyRead（~/.ssh 等凭证目录）不受影响。
	cfg.Filesystem.DenyWrite = filterConflictingDenyWrite(cfg.Filesystem.DenyWrite, cfg.Filesystem.WritableRoots)
	cfg.Network.Mode = agentbox.NetworkBlocked // 受限档默认=网络全拒（授权变体=D-2 网络族后续窗口）
	cfg.FallbackPolicy = agentbox.FallbackStrict // fail-closed——无静默降级（R-1368 同构纪律）
	mgr, err := agentbox.NewManager(cfg)
	if err != nil {
		return fmt.Errorf("%w: agentbox Manager 构造失败: %w", ErrNoBackend, err)
	}
	if !mgr.Available() {
		_ = mgr.Close()
		return fmt.Errorf("%w: agentbox 平台机制不可用（%s）", ErrNoBackend, p.platform)
	}
	p.mgr = mgr
	p.state = ProviderPrepared
	return nil
}

// Acquire 租约（句柄=Manager 共享引用+状态机——agentbox 进程生命周期内部承载）。
func (p *agentboxProvider) Acquire(_ context.Context, req LeaseRequest) (RuntimeHandle, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.state != ProviderPrepared {
		return nil, fmt.Errorf("runtime: Provider 未 Prepare（状态=%v）", p.state)
	}
	return &agentboxHandle{
		id:    fmt.Sprintf("ab-%d", time.Now().UnixNano()),
		p:     p,
		state: HandleAcquired,
	}, nil
}

// agentboxHandle agentbox 执行句柄（D-5 定序）。
type agentboxHandle struct {
	mu    sync.Mutex
	id    string
	p     *agentboxProvider
	state HandleState
}

func (h *agentboxHandle) ID() string { return h.id }
func (h *agentboxHandle) State() HandleState {
	h.mu.Lock()
	defer h.mu.Unlock()
	return h.state
}

// Start 建立边界（agentbox=每次 Exec 时包裹——Start=机制就绪确认点，无外部资源物化）。
func (h *agentboxHandle) Start(context.Context) error {
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.state != HandleAcquired {
		return ErrInvalidState
	}
	h.state = HandleReady
	return nil
}

// Precheck 边界验证（真实探针——claimed 边界全验证）：
// ①fs 探针：写工作区外=agentbox fs 禁闭拒绝；②网络探针：NetworkBlocked=出站拒绝。
// 反虚假绿：探针必须被子进程级拒绝（非分类器/构造错误——ExitCode 非零+错误证据文本）。
func (h *agentboxHandle) Precheck(ctx context.Context) error {
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.state != HandleReady && h.state != HandleRunning {
		return ErrInvalidState
	}
	// ①fs 探针：写工作区外路径
	res, err := h.p.mgr.ExecArgs(ctx, probeWriteBin(), probeWriteArgs())
	if err != nil {
		return fmt.Errorf("runtime: Precheck fs 探针执行失败: %w", err)
	}
	if res.ExitCode == 0 {
		return fmt.Errorf("runtime: Precheck fs 探针未被 fs 禁闭拒绝（写工作区外成功=边界失效）——stdout=%q", res.Stdout)
	}
	// ②网络探针：出站连接必须被拒
	res2, err := h.p.mgr.ExecArgs(ctx, probeNetBin(), probeNetArgs())
	if err != nil {
		return fmt.Errorf("runtime: Precheck 网络探针执行失败: %w", err)
	}
	if res2.ExitCode == 0 {
		return fmt.Errorf("runtime: Precheck 网络探针未被拒绝（出站成功=NetworkBlocked 失效）——stdout=%q", res2.Stdout)
	}
	h.state = HandleRunning
	return nil
}

// Execute 边界内执行（agentbox ExecArgs——分类器透传=治理归 GoalOS）。
func (h *agentboxHandle) Execute(ctx context.Context, req ExecuteRequest) (ExecuteResult, error) {
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
	res, err := h.p.mgr.ExecArgs(ctx, binary, strings.Fields(req.Params["args"]))
	out := ExecuteResult{Output: res_stdout(res, err), Cost: time.Since(start)}
	if err != nil {
		out.Status = "failed"
		out.ExitCode = -1
		out.ErrorCode = "agentbox_exec"
		return out, nil
	}
	out.Status = statusFromExit(res.ExitCode)
	out.ExitCode = res.ExitCode
	return out, nil
}

// res_stdout 输出提取（err 时 res 可能为 nil）。
func res_stdout(res *agentbox.ExecResult, err error) string {
	if res == nil {
		return fmt.Sprint(err)
	}
	if res.Stderr != "" && res.ExitCode != 0 {
		return res.Stdout + res.Stderr
	}
	return res.Stdout
}

// filterConflictingDenyWrite 剔除与 WritableRoots 冲突的 DenyWrite 条目。
// 冲突判定与 agentbox 校验器同规则（config.go validateFilesystem）：
// deny==root 或 deny 是 root 的前缀祖先 → 该 deny 条目使配置非法，剔除；
// 其余条目（Windows=系统目录族）原样保留。Linux/macOS 默认 deny 表不含 home
// →对本平台为无操作。
func filterConflictingDenyWrite(deny, roots []string) []string {
	kept := make([]string, 0, len(deny))
	for _, d := range deny {
		conflict := false
		for _, r := range roots {
			absD, absR := d, r
			if !filepath.IsAbs(absD) {
				absD, _ = filepath.Abs(absD)
			}
			if !filepath.IsAbs(absR) {
				absR, _ = filepath.Abs(absR)
			}
			if absR == absD || strings.HasPrefix(absR, absD+string(filepath.Separator)) {
				conflict = true
				break
			}
		}
		if !conflict {
			kept = append(kept, d)
		}
	}
	return kept
}

func statusFromExit(code int) string {
	if code == 0 {
		return "success"
	}
	return "failed"
}

// Interrupt 优雅取消（agentbox ctx 取消链——进程组清理归 agentbox 内部机制）。
func (h *agentboxHandle) Interrupt(context.Context) error {
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.state != HandleRunning {
		return ErrInvalidState
	}
	// agentbox 的进程生命周期=ctx 取消链承载（Execute 的 ctx 由调用方持有）
	return nil
}

// Pause 内核强制挂起（agentbox 无 Pause API——句柄级挂起：状态阻断 Execute 合法态；
// 进程级挂起=平台机制归 agentbox 路线图上流评估——诚实标注不假装）。
func (h *agentboxHandle) Pause(context.Context) error {
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.state != HandleRunning {
		return ErrInvalidState
	}
	h.state = HandlePaused
	return nil
}

// Resume 恢复。
func (h *agentboxHandle) Resume(context.Context) error {
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.state != HandlePaused {
		return ErrInvalidState
	}
	h.state = HandleRunning
	return nil
}

// Release 清理（幂等——Manager 生命周期归 Provider.Close）。
func (h *agentboxHandle) Release(context.Context) error {
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.state == HandleReleased || h.state == HandleDestroyed {
		return nil
	}
	h.state = HandleReleased
	return nil
}
