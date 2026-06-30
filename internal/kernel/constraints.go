// Package kernel 定义 GoalOS Kernel 的架构约束接口。
//
// 这些接口不是功能接口——它们是架构约束的编译器级强制执行。
// 违反这些约束 = 编译失败 = 架构违规被阻止。
//
// 设计依据:
//   会议 #81 R-482: 根源修复——架构约束从自然语言文档迁移到 Go 编译器
//   会议 #84 R-490: PluginRunner 显式 interface 提取
//   会议 #84 R-495: EventBus 核心层 Invariant (handler < 1ms)
//   会议 #84 R-499: pkg/events → goalos/events 路径迁移
//   会议 #84 R-500: EventPayload interface 统一事件验证
//
// 使用方式:
//   每个 internal/ 下的 package 必须实现 Layered 接口。
//   CI 静态分析 pass (R-496, Week 4) 扫描 import 路径——低层 import 高层 → 编译失败。
//   在静态分析就位前，constraints.go 的 interface 提供编译期类型安全检查。
package kernel

// ─── 约束 1: Event 边界 ───

// Event 是 Kernel 理解的系统事件基本结构。
// 定义在 kernel 包而非 events 包——消除循环依赖。
// 具体事件类型的 payload 定义在 goalos/events/ (R-499)。
type Event struct {
	Type      string
	GoalID    string
	Timestamp string
	Payload   interface{}
}

// StateChangeNotifier 是跨 package 状态变更的唯一出口。
// 任何 package 需要通知其他 package 状态变更时，必须通过此接口。
// 禁止跨 package 直接修改对方的状态。
//
// 设计依据: 产品原则 2 "Event over Call" + 会议 #79 R-452 + 会议 #80 F5。
// 边界标准: 跨 Go package 状态变更 → EventBus。同 package 内部可直接调用。
type StateChangeNotifier interface {
	// PublishStateChange 发布状态变更事件。仅用于跨 package 通信。
	// 同 package 内部调用不通过此接口。
	PublishStateChange(evt Event)
}

// ─── 约束 2: 分层依赖 ───

// Layer 定义架构分层。数字越小，层级越高。
// 上层可以依赖下层。下层禁止依赖上层。
//
// 分层铁律 (会议 #76 A1):
//   Dashboard(0) → API(1) → Scheduler(2) → GoalRunner(3) → PipelineRunner(4) → PluginRunner(5)
//   EventBus = 横向通道。MissionEngine/Governance/ContextEngine 作为同级模块通过 EventBus 通信。
type Layer int

const (
	LayerDashboard      Layer = iota // 0: Dashboard / Web UI
	LayerAPI                         // 1: HTTP API (daemon/api.go)
	LayerScheduler                   // 2: Scheduler (Goal 级事件路由)
	LayerGoalRunner                  // 3: GoalRunner (Goal 生命周期管理)
	LayerPipelineRunner              // 4: PipelineRunner (Action 级三原语执行)
	LayerPluginRunner                // 5: PluginRunner (子进程启动/IPC/生命周期)
)

// Layered 是所有架构分层模块必须实现的接口。
// CI lint 规则 (R-496, Week 4) 检查：模块只能 import 同层或更低层的 package。
type Layered interface {
	// Layer 返回此模块的架构层级。
	Layer() Layer
}

// ─── 约束 3: Primitive 边界 (会议 #78 第一性原理 + #79 E1) ───

// Primitive 是 Kernel 理解的原子操作。
// 当前只有 3 种：Check、Exec、Decide。
// Wait 不是 Primitive——它是 PipelineRunner 的中间状态。
type Primitive interface {
	// PrimitiveName 返回原语名称："Check" | "Exec" | "Decide"
	PrimitiveName() string
}

// CheckPrimitive 验证原语：暂停执行流→评估→返回 PASS/WARN/BLOCK/REJECT。
// 接口在 Kernel，实现在 Governance (PolicyGate/RiskGate) 和 ContextEngine (QualityGate)。
type CheckPrimitive interface {
	Primitive
	// Check 评估 Action 的准入条件。返回 PASS/WARN/BLOCK/REJECT 之一。
	Check(actionID string) CheckResult
}

// ExecPrimitive 执行原语：启动 Plugin 子进程→执行实际操作→产出 Artifact。
// R-490: 当前代码中 exec() 为空壳。Week 2 A8 修复后命名为 requestExecution。
type ExecPrimitive interface {
	Primitive
	// RequestExecution 提交 Action 到 PluginRunner 异步执行。
	// 发布 ActionExecutionRequested 事件。PluginRunner 订阅后启动子进程。
	// 返回的 Result 由 PluginRunner 通过 EventBus 回传。
	// R-503: session_token 使用 crypto/rand 生成，32 字节，entropy ≥ 256 bit。
	RequestExecution(actionID string) error
}

// DecidePrimitive 决策原语：分析失败→选择路径（CONTINUE/RETRY/REPLAN/ESCALATE/ABORT）。
// R-504: 超时副作用决策表——Exec 超时时根据 idempotency_token 状态选择路径。
type DecidePrimitive interface {
	Primitive
	// Decide 分析执行结果，选择下一步路径。
	// R-502: Multi-LLM WARN 决策矩阵覆盖 4×4=16 种组合。
	Decide(actionID string, execErr error) DecidePath
}

// CheckResult 是 Check 原语的返回枚举。
// 合法值: PASS, WARN, BLOCK, REJECT (B4: 枚举值验证)
type CheckResult string

const (
	CheckPASS   CheckResult = "PASS"
	CheckWARN   CheckResult = "WARN"
	CheckBLOCK  CheckResult = "BLOCK"
	CheckREJECT CheckResult = "REJECT"
)

// DecidePath 是 Decide 原语的路径枚举。
// 合法值: CONTINUE, RETRY, REPLAN, ESCALATE, ABORT (B3: 常量化)
type DecidePath string

const (
	DecideCONTINUE  DecidePath = "CONTINUE"
	DecideRETRY     DecidePath = "RETRY"
	DecideREPLAN    DecidePath = "REPLAN"
	DecideESCALATE  DecidePath = "ESCALATE"
	DecideABORT     DecidePath = "ABORT"
)

// ─── 约束 4: Projection 只读 ───

// Projection 是只读状态视图。
// 状态从 events.jsonl 推导（Projection over State 原则 4）。
// 禁止 Projection 修改业务状态或发布领域决策事件。
type Projection interface {
	// IsProjection 标记此类型为只读投影。
	IsProjection() bool
}

// ─── 约束 5: PluginRunner 显式接口 (会议 #84 R-490) ───

// PluginRunner 是 daemon 与 Plugin 子进程之间的桥。
// 订阅 ActionExecutionRequested → os/exec 启动子进程 → 返回 ResultMessage → 发布 ActionCompleted/ActionFailed。
//
// 设计依据: Week 0 提取为显式 Go interface。Mock 和 Real 都实现此 interface。
// TestIntegration_ExecRedTest 用 Mock 验证当前 exec() 空壳 (RED)→Week 2 A8 修复 (GREEN)。
type PluginRunner interface {
	Layered // PluginRunner 属于 Layer 5 (最底层)

	// Execute 启动 Plugin 子进程执行 Action。
	// sessionToken 由 PipelineRunner 通过 crypto/rand 生成 (R-503)。
	// 子进程通过 FD3 控制通道通信 (会议 #80 F1)。
	// 返回的 Result 由 PluginRunner 通过 EventBus 以 ActionCompleted/ActionFailed 事件发布。
	// 超时→SIGTERM→5s→SIGKILL (R-504)。
	// macOS: 使用 sandbox-exec + .sb 规则文件 (R-493)。
	// Linux: 使用 seccomp BPF (R-492)。
	// Execute(ctx context.Context, action Action, sessionToken []byte) (*Result, error)
}

// ─── 约束 6: EventBus 核心层 Invariant (会议 #84 R-495) ───

// CoreLayerHandler 标记 EventBus 核心层的同步 handler。
// 核心层 handler 必须在 < 1ms 内返回 (P95)。
// 禁止 I/O、禁止锁等待、禁止网络调用、禁止启动 goroutine。
//
// L5 基准测试: TestBenchmark_EventBusCoreLayerLatency 验证 P95 < 1ms。
// 违反 → CI 失败。
type CoreLayerHandler interface {
	// IsCoreLayerHandler 标记此 handler 运行在 EventBus 核心层。
	IsCoreLayerHandler() bool
}

// ─── 约束 7: Hot Path 标注 (会议 #84 R-491) ───

// HotPath 标记 PipelineRunner/GoalRunner/Scheduler 中的关键执行路径。
// Hot path 中的 EventBus Publish 必须显式标注目标层:
//   - 状态变更事件 (ActionStarted/Completed/Failed, GoalCompleted/Failed, PipelinePaused/Resumed)
//     → 核心层同步 (handler < 1ms)。
//   - 审计/指标/通知事件 (TokenUsage, Metrics, Notification, CheckPerformed, GateEvaluated)
//     → 副作用层异步 (goroutine pool)。
//
// 设计依据: 05-架构 §2.2 EventBus 两层架构 + S4 标准补充。
type HotPath interface {
	// IsHotPath 标记此代码路径为 hot path。
	// Hot path 中的操作必须标注延迟特性 (O(1), < 1ms)。
	IsHotPath() bool
}

// ─── 约束 8: EventPayload 统一验证 (会议 #84 R-500) ───

// EventPayload 是所有事件 payload 的统一接口。
// 每个事件类型必须实现此接口。
// M1-M8 契约闸口在 Validate() 中统一验证。
//
// 定义位置: goalos/events/payload.go (R-499: pkg/events → goalos/events)
// 实现时机: Week 5 B2 typed event payload 迁移。
//
// 示例:
//   type GoalCreatedPayload struct { Title string `json:"title"` }
//   func (p GoalCreatedPayload) EventType() string { return "GoalCreated" }
//   func (p GoalCreatedPayload) Validate() error {
//       if p.Title == "" { return fmt.Errorf("GoalCreatedPayload: Title is required (M4)") }
//       return nil
//   }
type EventPayload interface {
	// EventType 返回事件类型字符串。必须与 07-事件注册表中的 type 字段一致。
	EventType() string
	// Validate 执行 M1-M8 契约闸口验证。验证失败 → 拒绝发布事件。
	Validate() error
}
