//go:build linux

// provider_linux.go——Linux 受限档双引擎收敛（R-1664——会议 #267 顾问第七~九轮）：
// 能力探测（实证式——不问 agentbox Available() 静态声明）：
//   模式 A=agentbox（namespace 族——userns 可用环境）；
//   模式 B=免 userns 原生沙箱（landlock+seccomp——Ubuntu 24.04 AppArmor 默认环境）；
//   双不可用=fail-closed（ErrNoBackend——诚实报错非裸跑）。
package runtime

import (
	"context"
	"fmt"
	"os"

	"github.com/goalos/goalos/internal/fd3"
)

// dualEngineProvider 双引擎 Provider（Prepare 期实证选择引擎——选择记录可观测）。
type dualEngineProvider struct {
	workspace string
	tmpDir    string
	engine    Provider // Prepare 期选定
	mode      string   // "A"(agentbox)/"B"(modeb)——可观测性
	dialFn    fd3.DialFunc
	onDeny    func(endpoint, reason string)
}

// LinuxOption 双引擎构造可选项（R-1650 v2 FD3 接线面——仅模式 B 消费）。
type LinuxOption func(*dualEngineProvider)

// WithLinuxDialFunc 注入模式 B broker 拨号面（生产=zone dialer 同源）。
func WithLinuxDialFunc(d fd3.DialFunc) LinuxOption {
	return func(p *dualEngineProvider) { p.dialFn = d }
}

// WithLinuxOnDeny 注入 broker 拒绝审计回调。
func WithLinuxOnDeny(fn func(endpoint, reason string)) LinuxOption {
	return func(p *dualEngineProvider) { p.onDeny = fn }
}

// NewLinuxRestrictedProvider Linux 受限档唯一构造入口（双引擎收敛——
// 生产接线与 TC-RT-001b 同源）。
func NewLinuxRestrictedProvider(workspace, tmpDir string, opts ...LinuxOption) Provider {
	p := &dualEngineProvider{workspace: workspace, tmpDir: tmpDir}
	for _, opt := range opts {
		opt(p)
	}
	return p
}

func (p *dualEngineProvider) Name() string { return "linux-dual" }
func (p *dualEngineProvider) Tier() string { return "T1" }

func (p *dualEngineProvider) State(ctx context.Context) (ProviderState, error) {
	if p.engine == nil {
		return ProviderRegistered, nil
	}
	return p.engine.State(ctx)
}

func (p *dualEngineProvider) Capabilities(ctx context.Context) (ProviderCapability, error) {
	if p.engine == nil {
		return ProviderCapability{Platform: "linux"}, nil
	}
	c, err := p.engine.Capabilities(ctx)
	if err == nil {
		c.Platform = "linux-" + p.mode // 引擎显式化（降级证据显式化——R-1506 同构）
	}
	return c, err
}

// Prepare 引擎实证选择：
//  1. 模式 A 实证——agentbox 管理器构造+真实探针（经 agentbox 沙箱写 home——
//     拒绝=边界在位；泄漏/错误=userns 被拦族，不可信）；
//  2. 模式 B 实证——landlock ABI+架构探测（modeBAvailable）；
//  3. 双不可用=fail-closed。
func (p *dualEngineProvider) Prepare(ctx context.Context, plan RuntimePlan) error {
	// 模式 A 实证（agentbox——empirical，不问 Available()）
	agent := NewAgentboxProvider(p.workspace, p.tmpDir, "linux")
	if err := agent.Prepare(ctx, plan); err == nil {
		if p.probeEngineA(ctx, agent) {
			p.engine, p.mode = agent, "A"
			return nil
		}
	}
	// 模式 B 实证（免 userns——landlock ABI+架构）
	if modeBAvailable() {
		mb := NewModeBProvider(p.workspace, p.tmpDir,
			WithModeBDialFunc(p.dialFn), WithModeBOnDeny(p.onDeny))
		if err := mb.Prepare(ctx, plan); err == nil {
			p.engine, p.mode = mb, "B"
			return nil
		}
	}
	return fmt.Errorf("%w: 双引擎均不可用（模式 A=userns 被拦族/模式 B=landlock 缺席）——受限档本平台不可用（fail-closed）", ErrNoBackend)
}

// probeEngineA 模式 A 边界实证——经 agentbox 沙箱跑 home 写探针：
// 拒绝=边界在位（真 A）；写成功=fail-open 裸跑（弃用转 B）。
func (p *dualEngineProvider) probeEngineA(ctx context.Context, agent Provider) bool {
	h, err := agent.Acquire(ctx, LeaseRequest{GoalID: "engine-probe", ActionID: "engine-probe"})
	if err != nil {
		return false
	}
	defer h.Release(context.Background())
	if err := h.Start(ctx); err != nil {
		return false
	}
	// Precheck=边界实证（home 写探针——S-266-01 强化版）；通过=模式 A 可信
	return h.Precheck(ctx) == nil
}

func (p *dualEngineProvider) Acquire(ctx context.Context, req LeaseRequest) (RuntimeHandle, error) {
	if p.engine == nil {
		return nil, fmt.Errorf("%w: 引擎未选择（Prepare 未调用或失败）", ErrNoBackend)
	}
	h, err := p.engine.Acquire(ctx, req)
	if err != nil {
		return nil, err
	}
	// 模式 B 激活时的诚实声明（九轮——抽象套接字面开放）
	if p.mode == "B" {
		fmt.Fprintf(os.Stderr, "[goalos] 模式 B（免 userns）激活：网络隔离仅阻止 IP 协议族（AF_INET/INET6），不防御依赖宿主抽象套接字的本地提权攻击（CVE-2020-15257 族）\n")
	}
	return h, nil
}
