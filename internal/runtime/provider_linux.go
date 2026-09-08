//go:build linux

// provider_linux.go——Linux 受限档双引擎收敛（R-1664 框架+R-1679 换引擎——会议 #280 续）：
// 能力探测（实证式——不问静态声明）：
//   模式 A=bwrap（userns+mount ns 遮蔽——隔离强度更高=文件系统视图级；
//         依赖=bwrap 二进制+Ubuntu 24.04 需 goalos-bwrap profile）；
//   模式 B=免 userns 原生沙箱（landlock+seccomp——零依赖兜底线）；
//   双不可用=fail-closed（ErrNoBackend——诚实报错非裸跑）。
// agentbox 模式 A 位已退役（R-1678——递归爆炸+fail-open 实机事故链）。
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
	mode      string   // "A"(bwrap)/"B"(modeb)——可观测性
	dialFn    fd3.DialFunc
	onDeny    func(endpoint, reason string)
}

// LinuxOption 双引擎构造可选项（R-1650 v2 FD3 接线面——引擎均可消费）。
type LinuxOption func(*dualEngineProvider)

// WithLinuxDialFunc 注入 broker 拨号面（生产=zone dialer 同源）。
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
//  1. 模式 A 实证——bwrap（Prepare 探针=实证 spawn+Precheck 边界实证）；
//  2. 模式 B 实证——landlock ABI+架构探测（modeBAvailable）；
//  3. 双不可用=fail-closed。
func (p *dualEngineProvider) Prepare(ctx context.Context, plan RuntimePlan) error {
	// 模式 A 实证（bwrap——ns 级隔离优先；Prepare 失败/边界实证不过=落 B）
	bw := NewBwrapProvider(p.workspace, p.tmpDir,
		WithBwrapDialFunc(p.dialFn), WithBwrapOnDeny(p.onDeny))
	if err := bw.Prepare(ctx, plan); err == nil {
		if p.probeEngineA(ctx, bw) {
			p.engine, p.mode = bw, "A"
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
	return fmt.Errorf("%w: 双引擎均不可用（模式 A=bwrap 缺席或实证失败/模式 B=landlock 缺席）——受限档本平台不可用（fail-closed）", ErrNoBackend)
}

// probeEngineA 模式 A 边界实证——经 bwrap 沙箱跑 home 写探针：
// 拒绝=边界在位（真 A）；写成功/错误=fail-open 疑面（弃用转 B）。
func (p *dualEngineProvider) probeEngineA(ctx context.Context, engine Provider) bool {
	h, err := engine.Acquire(ctx, LeaseRequest{GoalID: "engine-probe", ActionID: "engine-probe"})
	if err != nil {
		return false
	}
	defer h.Release(context.Background())
	if err := h.Start(ctx); err != nil {
		return false
	}
	// Precheck=边界实证（home 写探针+出站探针——S-266-01 强化版）；通过=模式 A 可信
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
