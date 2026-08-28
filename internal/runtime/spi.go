// spi.go——Provider SPI v2 全量（任务 3.2；接口定稿权威=05 §X.6.5——R-1523/R-1615；
// 方法级行为契约=R-1620 契约表（05 §X.6.5）；支撑类型全量=R-1608/D-13；
// 骨架纪律=R-1468：未实现不注册——RegisterChecked 注册时探测拒绝骨架）。
package runtime

import (
	"context"
	"errors"
	"time"
)

// ─── 错误值（09 RTM 族注册注记——R-1620）───

// ErrInvalidState 非法状态调用（方法级契约表——前置状态不满足）。
var ErrInvalidState = errors.New("runtime: 非法状态调用（前置状态不满足——R-1620 契约表）")

// ErrHandleReleased 句柄已释放（Release 后任何调用）。
var ErrHandleReleased = errors.New("runtime: 句柄已释放（Release 后调用——R-1620）")

// ErrNoBackend 无可用后端（平台探测无匹配——R-1599 诚实降级语境）。
var ErrNoBackend = errors.New("runtime: 无可用后端")

// ErrNotImplemented 骨架占位（R-1468 统一纪律——骨架函数唯一合法返回值）。
var ErrNotImplemented = errors.New("runtime: 未实现（骨架——R-1468）")

// ─── 三枚举（显式赋值从 1 开始——零值非法，D-13）───

// ProviderState Provider 生命周期状态（05 §X.6.7）。
type ProviderState int

const (
	// ProviderRegistered 已注册（值=1——零值非法）
	ProviderRegistered ProviderState = iota + 1
	ProviderPrepared                  // Prepare 完成
	ProviderDegraded                  // 降级（部分能力不可用但核心机制有效——证据入 ProviderCapability）
	ProviderDisposed                  // 已销毁
)

// HandleState 租约句柄状态机（05 §X.6.6——D-5 定序：Acquire→Start→Precheck→Running）。
type HandleState int

const (
	// HandleAcquired 已获取（边界资源已占——值=1，零值非法）
	HandleAcquired HandleState = iota + 1
	HandleReady                // Start 完成（边界已建立）
	HandleRunning              // Precheck 通过（边界已验证——Execute 唯一合法态）
	HandlePaused               // 已挂起（SIGSTOP 族——R-1245）
	HandleReleased             // 已释放（热池归还或销毁）
	HandleDestroyed            // 强制销毁（升级失败/预检失败——不留热池）
)

// ─── 支撑类型全量定义（D-13——字段全量，非注释桩）───

// LeaseRequest 租约请求（08 §19.10a 映射：Contract=VerifiedContract 仅验证层可构造；
// Deadline 用于热池预算判定）。
type LeaseRequest struct {
	Contract *VerifiedContract // 已验证契约（唯一构造=契约验证层——R-1501 契约不过边界）
	GoalID   string
	ActionID string
	Deadline time.Duration // Acquire 预算（热池命中/冷启动超时上限——TC-RT-060 分段归因数据源）
}

// RuntimePlan Prepare 用计划（profile 编译产物引用+资源需求）。
type RuntimePlan struct {
	PlanID               string
	Tier                 ExecutionTier
	ProfileDigest        string   // hex(32B)——生效 CompiledProfile 摘要（租约前重算比对）
	RequiredCapabilities []string // 契约声明能力集
}

// ProviderCapability Provider 能力快照（降级证据显式化——R-1506）。
type ProviderCapability struct {
	Platform          string         // darwin/linux/windows/xinchuang
	AchievedIsolation IsolationLevel // 本平台实际达成 I 级（探测+验证——R-1544）
	DegradedEvidence  []string       // 降级证据（如 cgroup v1 而非 v2）——空=无降级
	WarmPool          bool           // 热池（VMSnapshot）能力
}

// ExecuteRequest 执行请求（Action 粒度——R-1504）。
type ExecuteRequest struct {
	ActionID   string
	ActionType string            // capability 动词（fs.write/shell.execute 等）
	Params     map[string]string // 执行参数（大 payload 走 data_ref——FD3 纪律）
	Timeout    time.Duration     // Action 超时（默认 30s——超时→SIGTERM→2s→SIGKILL，R-1150）
}

// ExecuteResult 执行结果。
type ExecuteResult struct {
	Status    string        // "success"|"failed"|"timeout"|"cancelled"
	ExitCode  int           // 进程退出码（-1=非进程语义）
	Output    string        // 文本输出（大产出=data_ref 引用）
	ErrorCode string        // 09 错误码（空=无错误）
	Cost      time.Duration // 执行耗时（分段归因数据源——TC-RT-060）
	Artifacts []string      // 产出物清单（MustHave 校验数据源）
}

// SnapshotRef 快照引用（VMSnapshot 热池镜像——R-1495）。
type SnapshotRef struct {
	ID        string
	Provider  string
	CreatedAt time.Time
	Digest    string // 快照内容摘要（恢复完整性校验）
}

// WorkspaceRef 工作区卷引用（升级=新会话挂同卷——R-1499）。
type WorkspaceRef struct {
	VolumeID string // 卷标识（跨会话不变）
	Path     string // 挂载路径
}

// ─── SPI v2 接口（05 §X.6.5——同步 Go 接口+ctx，R-1500）───

// Provider 运行时提供方（生命周期/能力面）。
type Provider interface {
	Name() string
	Tier() string // wire 值=T0|T1|T2|T3（ExecutionTier.String()）
	State(context.Context) (ProviderState, error)
	Capabilities(context.Context) (ProviderCapability, error)
	Prepare(context.Context, RuntimePlan) error // 一次性准备（探测依赖/校验平台前提）；幂等
	Acquire(context.Context, LeaseRequest) (RuntimeHandle, error)
}

// RuntimeHandle 执行句柄（一次租约的执行面——D-5 定序：Start→Precheck→Execute；
// 方法级契约=R-1620 契约表）。
type RuntimeHandle interface {
	ID() string
	Precheck(context.Context) error                       // 前置=Ready；幂等重验允许
	Start(context.Context) error                          // 前置=Acquired；Ready 重复=ErrInvalidState
	Execute(context.Context, ExecuteRequest) (ExecuteResult, error) // 仅 Running
	Interrupt(context.Context) error                      // 仅 Running；child 已退出=成功
	Pause(context.Context) error                          // 仅 Running；不幂等
	Resume(context.Context) error                         // 仅 Paused
	Release(context.Context) error                        // 任意态；二次=幂等成功
	State() HandleState                                   // 全态可调
}

// Snapshottable 可选接口（类型断言检测——禁止空实现 R-1468；VMSnapshot 热池镜像=R-1495）。
type Snapshottable interface {
	Snapshot(context.Context) (SnapshotRef, error)
	RestoreFrom(context.Context, SnapshotRef) (RuntimeHandle, error)
}

// ─── HandleGuard 方法级契约执行（R-1620 契约表——包装器统一守门）───

// HandleGuard RuntimeHandle 契约守门包装器（前置状态校验+释放后封闭）。
type HandleGuard struct {
	inner RuntimeHandle
}

// NewHandleGuard 包装句柄（ExecutionSession 创建时统一包装——契约表唯一执行点）。
func NewHandleGuard(inner RuntimeHandle) *HandleGuard { return &HandleGuard{inner: inner} }

// ID 透传。
func (g *HandleGuard) ID() string { return g.inner.ID() }

// State 全态可调（Released 返回 HandleReleased 值非错误——R-1620）。
func (g *HandleGuard) State() HandleState { return g.inner.State() }

// Precheck 前置=Ready/Running（Running=幂等重验允许——验证只读；Paused=非法——挂起态不重验）。
func (g *HandleGuard) Precheck(ctx context.Context) error {
	switch g.inner.State() {
	case HandleReleased, HandleDestroyed:
		return ErrHandleReleased
	case HandleReady, HandleRunning:
		return g.inner.Precheck(ctx)
	default:
		return ErrInvalidState
	}
}

// Start 前置=Acquired（边界只建一次——Ready 重复=ErrInvalidState）。
func (g *HandleGuard) Start(ctx context.Context) error {
	switch g.inner.State() {
	case HandleReleased, HandleDestroyed:
		return ErrHandleReleased
	case HandleAcquired:
		return g.inner.Start(ctx)
	default:
		return ErrInvalidState
	}
}

// Execute 仅 Running（R-1608/D-5——预检闸门不可架空）。
func (g *HandleGuard) Execute(ctx context.Context, req ExecuteRequest) (ExecuteResult, error) {
	switch g.inner.State() {
	case HandleReleased, HandleDestroyed:
		return ExecuteResult{}, ErrHandleReleased
	case HandleRunning:
		return g.inner.Execute(ctx, req)
	default:
		return ExecuteResult{}, ErrInvalidState
	}
}

// Interrupt 仅 Running（child 已自然退出=成功——归 Provider 语义层，Guard 不拦截 Running 态调用）。
func (g *HandleGuard) Interrupt(ctx context.Context) error {
	switch g.inner.State() {
	case HandleReleased, HandleDestroyed:
		return ErrHandleReleased
	case HandleRunning:
		return g.inner.Interrupt(ctx)
	default:
		return ErrInvalidState
	}
}

// Pause 仅 Running（不幂等）。
func (g *HandleGuard) Pause(ctx context.Context) error {
	switch g.inner.State() {
	case HandleReleased, HandleDestroyed:
		return ErrHandleReleased
	case HandleRunning:
		return g.inner.Pause(ctx)
	default:
		return ErrInvalidState
	}
}

// Resume 仅 Paused。
func (g *HandleGuard) Resume(ctx context.Context) error {
	switch g.inner.State() {
	case HandleReleased, HandleDestroyed:
		return ErrHandleReleased
	case HandlePaused:
		return g.inner.Resume(ctx)
	default:
		return ErrInvalidState
	}
}

// Release 任意态可调；二次=幂等成功（与 IsolationBackend.Destroy 幂等纪律一致——R-1620）。
func (g *HandleGuard) Release(ctx context.Context) error {
	switch g.inner.State() {
	case HandleReleased, HandleDestroyed:
		return nil // 幂等成功
	}
	return g.inner.Release(ctx)
}

// ─── 注册时骨架拒绝（R-1468/TC-RT-090）───

// RegisterChecked 注册前探测（骨架=State 返回 ErrNotImplemented→拒绝注册）。
func (r *ProviderRegistry) RegisterChecked(p Provider) error {
	if _, err := p.State(context.Background()); errors.Is(err, ErrNotImplemented) {
		return ErrNotImplemented
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	r.byTier[p.Tier()] = append(r.byTier[p.Tier()], providerHandleAdapter{p})
	return nil
}

// providerHandleAdapter Provider→ProviderHandle 身份适配（注册表 W1 身份接口不动）。
type providerHandleAdapter struct{ p Provider }

func (a providerHandleAdapter) Name() string { return a.p.Name() }
func (a providerHandleAdapter) Tier() string { return a.p.Tier() }
