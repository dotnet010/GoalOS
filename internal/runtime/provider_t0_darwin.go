//go:build darwin

// provider_t0_darwin.go——协作档（T0）Provider（任务 5.4——R-1478②/R-1479）。
// T0 语义（05 §X.6.4 行 1+R-1619）：名单登记工作负载（trusted_workloads 命中=管理员显式
// 登记的信任决策，非发行者密码学认证）+能力代理语义——Execute 仅放行契约声明能力集
// （声明集外=能力代理拒绝——非 OS 边界拒绝，BypassCounters 分离计数 TC-RT-002 联动）。
// v0.3.1 诚实边界：本版本无生产名单工作负载（GoalAgent 不在名单——R-1507），
// T0 档仅测试夹具可达（05 §X.6.3 现状注记）。
package runtime

import (
	"context"
	"fmt"
)

// collaborativeHandle 协作档句柄（能力代理承载——OS 边界=seatbelt 变体复用受限档机制）。
type collaborativeHandle struct {
	inner       *seatbeltHandle      // OS 边界复用受限档机制（单源纪律）
	declaredCap map[string]bool      // 契约声明能力集（能力代理白名单——T0 语义核心）
	counters    *BypassCounters      // 代理拒绝计数（分离纪律——代理拒绝≠边界拒绝）
}

// darwinCollaborativeProvider 协作档 Provider（T0——能力代理+会话凭据语义）。
type darwinCollaborativeProvider struct {
	base *darwinSeatbeltProvider // 平台机制复用（单源——禁止第二份 seatbelt 调用）
}

// NewDarwinCollaborativeProvider 构造协作档 Provider（T0）。
// 与受限档共享平台机制（seatbelt 边界——协作档=名单信任+能力代理，非无边界）。
func NewDarwinCollaborativeProvider(workspace, tmpDir string) Provider {
	return &darwinCollaborativeProvider{
		base: NewDarwinSeatbeltProvider(workspace, tmpDir).(*darwinSeatbeltProvider),
	}
}

func (p *darwinCollaborativeProvider) Name() string { return "darwin-collaborative" }
func (p *darwinCollaborativeProvider) Tier() string { return TierT0.String() }

func (p *darwinCollaborativeProvider) State(ctx context.Context) (ProviderState, error) {
	return p.base.State(ctx)
}

// Capabilities 协作档能力快照（T0=约定级信任——降级证据=名单认证非密码学的诚实标注）。
func (p *darwinCollaborativeProvider) Capabilities(ctx context.Context) (ProviderCapability, error) {
	caps, err := p.base.Capabilities(ctx)
	if err != nil {
		return caps, err
	}
	caps.DegradedEvidence = append(caps.DegradedEvidence,
		"协作档信任=管理员名单登记（非发行者密码学认证——R-1619 诚实标注）")
	return caps, nil
}

func (p *darwinCollaborativeProvider) Prepare(ctx context.Context, plan RuntimePlan) error {
	return p.base.Prepare(ctx, plan)
}

// Acquire 协作档租约（契约声明能力集=能力代理白名单数据源——契约必须非空：
// T0 的代理语义依赖声明集，无契约=fail-closed 拒绝租约）。
func (p *darwinCollaborativeProvider) Acquire(ctx context.Context, req LeaseRequest) (RuntimeHandle, error) {
	if req.Contract == nil {
		return nil, fmt.Errorf("runtime: 协作档租约必须携带已验证契约（能力代理白名单=声明集——无契约不租约，R-1501）")
	}
	inner, err := p.base.Acquire(ctx, req)
	if err != nil {
		return nil, err
	}
	declared := make(map[string]bool)
	for _, c := range req.Contract.Claims().Capabilities {
		declared[c] = true
	}
	return &collaborativeHandle{
		inner:       inner.(*seatbeltHandle),
		declaredCap: declared,
		counters:    NewBypassCounters(),
	}, nil
}

// ─── 能力代理面（T0 语义核心——声明集外执行=代理拒绝分离计数）───

func (h *collaborativeHandle) ID() string { return h.inner.ID() }
func (h *collaborativeHandle) State() HandleState { return h.inner.State() }
func (h *collaborativeHandle) Start(ctx context.Context) error { return h.inner.Start(ctx) }
func (h *collaborativeHandle) Precheck(ctx context.Context) error { return h.inner.Precheck(ctx) }

// Execute 能力代理执行（T0 语义）：动作能力 ∈ 契约声明集→放行（OS 边界内）；
// ∉声明集→能力代理拒绝（RecordProxyRefusal——不计入边界防线证据，TC-RT-002）。
func (h *collaborativeHandle) Execute(ctx context.Context, req ExecuteRequest) (ExecuteResult, error) {
	// 能力代理判定（契约声明集白名单——T0 协作档语义核心）
	capVerb := req.Params["capability"]
	if capVerb == "" {
		capVerb = req.ActionType
	}
	if !h.declaredCap[capVerb] {
		h.counters.RecordProxyRefusal(capVerb)
		return ExecuteResult{}, fmt.Errorf("runtime: 能力代理拒绝——%q 不在契约声明能力集（协作档 T0 代理语义）", capVerb)
	}
	// 能力动词=代理门面；OS 层执行形态=process.exec（边界内直接执行——动词与机制分层）
	innerReq := req
	innerReq.ActionType = "process.exec"
	return h.inner.Execute(ctx, innerReq)
}

// ProxyRefusals 代理拒绝计数（BypassCounters 分离计数读取点——TC-RT-002）。
func (h *collaborativeHandle) ProxyRefusals() int { return h.counters.ProxyRefusals }

func (h *collaborativeHandle) Interrupt(ctx context.Context) error { return h.inner.Interrupt(ctx) }
func (h *collaborativeHandle) Pause(ctx context.Context) error     { return h.inner.Pause(ctx) }
func (h *collaborativeHandle) Resume(ctx context.Context) error    { return h.inner.Resume(ctx) }
func (h *collaborativeHandle) Release(ctx context.Context) error   { return h.inner.Release(ctx) }

