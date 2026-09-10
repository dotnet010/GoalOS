// session_contract_test.go——任务 3.3 契约测试（R-571 先红；TC-RT-070 升级端到端三断言：
// 取消时序/Cancelled 标记/重执行不重复产出——规格=05 §X.6.6+R-1499/R-1504/R-1509/
// R-1515/R-1522/R-1525/R-1529；升级=优雅取消+新会话挂同卷，粒度=Action 1:1）。
package runtime

import (
	"fmt"
	"context"
	"testing"
)

// ─── TC-RT-070 夹具：可编程升级 Provider（T0→受限档升级端到端）───

// upgradeFakeProvider 记录 Interrupt/Release 时序的可编程 Provider。
type upgradeFakeProvider struct {
	name         string
	tier         string
	handle       *upgradeFakeHandle
	acquireCalls int
}

type upgradeFakeHandle struct {
	id            string
	state         HandleState
	interruptSeq  []string         // Interrupt 调用时序记录（取消协议链断言数据源）
	executed      []ExecuteRequest // 已执行 Action 序列（完整请求——R-1640-3 保真断言数据源）
	interruptErr  error
	releaseErr    error
}

func (p *upgradeFakeProvider) Name() string { return p.name }
func (p *upgradeFakeProvider) Tier() string { return p.tier }
func (p *upgradeFakeProvider) State(context.Context) (ProviderState, error) {
	return ProviderPrepared, nil
}
func (p *upgradeFakeProvider) Capabilities(context.Context) (ProviderCapability, error) {
	return ProviderCapability{Platform: "test", AchievedIsolation: I2}, nil
}
func (p *upgradeFakeProvider) Prepare(context.Context, RuntimePlan) error { return nil }
func (p *upgradeFakeProvider) Acquire(_ context.Context, req LeaseRequest) (RuntimeHandle, error) {
	p.acquireCalls++
	return p.handle, nil
}

func (h *upgradeFakeHandle) ID() string { return h.id }
func (h *upgradeFakeHandle) Precheck(context.Context) error {
	h.state = HandleRunning
	return nil
}
func (h *upgradeFakeHandle) Start(context.Context) error {
	h.state = HandleRunning
	return nil
}
func (h *upgradeFakeHandle) Execute(_ context.Context, req ExecuteRequest) (ExecuteResult, error) {
	h.executed = append(h.executed, req)
	return ExecuteResult{Status: "success", ExitCode: 0}, nil
}
func (h *upgradeFakeHandle) Interrupt(context.Context) error {
	// 取消协议链留痕（CancelMessage→SIGTERM→2s→SIGKILL——R-1107/R-1150；时序载体=调用记录）
	h.interruptSeq = append(h.interruptSeq, "Interrupt")
	if h.interruptErr != nil {
		return h.interruptErr
	}
	h.state = HandleReleased
	return nil
}
func (h *upgradeFakeHandle) Pause(context.Context) error { return nil }
func (h *upgradeFakeHandle) Resume(context.Context) error { return nil }
func (h *upgradeFakeHandle) Release(context.Context) error {
	if h.releaseErr != nil {
		return h.releaseErr
	}
	h.state = HandleReleased
	return nil
}
func (h *upgradeFakeHandle) State() HandleState { return h.state }

// TestRuntime_Escalate_NewSessionSameVolume（TC-RT-070——12 清单 F 节）：
// T0→受限档升级端到端——(1)取消时序（Interrupt 链被调用一次）(2)被中断 Action 标记 Cancelled
// (3)新会话挂同卷重执行该 Action（已完成 Action 产出物不重复、审计事件链完整——R-1499）。
func TestRuntime_Escalate_NewSessionSameVolume(t *testing.T) {
	ctx := context.Background()
	handle := &upgradeFakeHandle{id: "h-t0", state: HandleAcquired}
	provider := &upgradeFakeProvider{name: "fake-t0", tier: "T0", handle: handle}
	volume := WorkspaceRef{VolumeID: "vol-1", Path: "/tmp/ws-1"}

	// 建 T0 会话：Acquired→Start→Precheck→Running
	sess := NewExecutionSession("sess-1", "goal-1", volume)
	if err := sess.Attach(ctx, provider, LeaseRequest{GoalID: "goal-1", ActionID: "act-1"}); err != nil {
		t.Fatalf("Attach 失败: %v", err)
	}
	if sess.State() != SessionRunning {
		t.Fatalf("Attach 后应=Running，实际: %v", sess.State())
	}

	// 完成 act-1（产出物登记），act-2 执行中（升级信号到达时被中断）
	if _, err := sess.Execute(ctx, ExecuteRequest{ActionID: "act-1", ActionType: "fs.write"}); err != nil {
		t.Fatalf("act-1 执行失败: %v", err)
	}
	sess.MarkActionCompleted("act-1", []string{"out/artifact-1"})

	// 升级信号 → 升级到受限档（新 Provider=T1）
	handleT1 := &upgradeFakeHandle{id: "h-t1", state: HandleAcquired}
	providerT1 := &upgradeFakeProvider{name: "fake-t1", tier: "T1", handle: handleT1}
	sess.MarkActionExecuting(ExecuteRequest{ActionID: "act-2", ActionType: "shell.execute", Timeout: 5 * 1e9})

	newSess, err := sess.Escalate(ctx, providerT1, "capability_proxy")
	if err != nil {
		t.Fatalf("升级失败: %v", err)
	}

	// 断言(1)取消时序：Interrupt 被调用恰好一次（优雅取消协议链入口）
	if len(handle.interruptSeq) != 1 {
		t.Fatalf("断言(1)失败：Interrupt 应调用 1 次，实际 %d", len(handle.interruptSeq))
	}
	// 断言(2)被中断 Action 标记 Cancelled
	if got := sess.ActionStatus("act-2"); got != "Cancelled" {
		t.Fatalf("断言(2)失败：act-2 应=Cancelled，实际 %q", got)
	}
	// 断言(3)新会话挂同卷+重执行 act-2+已完成 act-1 不重复产出
	if newSess.Workspace().VolumeID != "vol-1" {
		t.Fatalf("断言(3)失败：新会话卷=%q 应=vol-1（同卷引用不变）", newSess.Workspace().VolumeID)
	}
	if err := newSess.ReexecuteInterrupted(ctx); err != nil {
		t.Fatalf("重执行失败: %v", err)
	}
	// act-1 不在新会话重执行（已完成产出物不重复）
	for _, r := range handleT1.executed {
		if r.ActionID == "act-1" {
			t.Fatal("断言(3)失败：已完成 act-1 在新会话被重复执行——产出物重复")
		}
	}
	found := false
	for _, r := range handleT1.executed {
		if r.ActionID == "act-2" {
			found = true
			// R-1640-3 请求保真断言：重执行=同一 Action（原 ActionType/Timeout 不丢——
			// 修复前硬编码 "reexecute" 占位的架空行为已消除）
			if r.ActionType != "shell.execute" || r.Timeout != 5*1e9 {
				t.Fatalf("断言(3)保真失败：act-2 重执行请求被篡改——ActionType=%q Timeout=%v（应=shell.execute/5s）", r.ActionType, r.Timeout)
			}
		}
	}
	if !found {
		t.Fatal("断言(3)失败：act-2 未在新会话重执行")
	}
	// 升级事件链：旧会话终态=已升级关闭
	if sess.State() != SessionEscalated {
		t.Fatalf("旧会话终态应=SessionEscalated，实际: %v", sess.State())
	}
	if newSess.State() != SessionRunning {
		t.Fatalf("新会话应=Running，实际: %v", newSess.State())
	}
}

// TestRuntime_Session_EscalateInterruptFailure 升级中旧会话清理失败=强制销毁+留痕
// （05 §X.6.6——重签失败=任务失败；旧会话清理失败=强制销毁+事件留痕）。
func TestRuntime_Session_EscalateInterruptFailure(t *testing.T) {
	ctx := context.Background()
	handle := &upgradeFakeHandle{id: "h-x", state: HandleAcquired, interruptErr: context.DeadlineExceeded}
	provider := &upgradeFakeProvider{name: "fake", tier: "T0", handle: handle}
	sess := NewExecutionSession("sess-x", "goal-x", WorkspaceRef{VolumeID: "vol-x", Path: "/tmp/x"})
	if err := sess.Attach(ctx, provider, LeaseRequest{GoalID: "goal-x", ActionID: "act-1"}); err != nil {
		t.Fatal(err)
	}
	sess.MarkActionExecuting(ExecuteRequest{ActionID: "act-1", ActionType: "fs.write"})

	_, err := sess.Escalate(ctx, &upgradeFakeProvider{name: "t1", tier: "T1", handle: &upgradeFakeHandle{id: "h-y", state: HandleAcquired}}, "risk_reeval")
	if err == nil {
		t.Fatal("Interrupt 失败的升级必须失败（重签/取消失败=任务失败）")
	}
	// 旧会话清理失败=强制销毁——状态=已销毁
	if sess.State() != SessionDestroyed {
		t.Fatalf("Interrupt 失败后旧会话应=强制销毁，实际: %v", sess.State())
	}
}

// ─── R-1640-1 夹具：Precheck 失败 Provider（P1——句柄清理断言）───

type precheckFailHandle struct {
	id            string
	state         HandleState
	releaseCalled bool
}

func (h *precheckFailHandle) ID() string { return h.id }
func (h *precheckFailHandle) Start(context.Context) error {
	h.state = HandleReady // D-5 定序：Start→Ready（Precheck 闸门不架空——R-1620）
	return nil
}
func (h *precheckFailHandle) Precheck(context.Context) error {
	return fmt.Errorf("runtime: 边界建立未生效（夹具模拟 Precheck 失败）")
}
func (h *precheckFailHandle) Execute(context.Context, ExecuteRequest) (ExecuteResult, error) {
	return ExecuteResult{}, fmt.Errorf("不应到达 Execute")
}
func (h *precheckFailHandle) Interrupt(context.Context) error { return nil }
func (h *precheckFailHandle) Pause(context.Context) error     { return nil }
func (h *precheckFailHandle) Resume(context.Context) error    { return nil }
func (h *precheckFailHandle) Release(context.Context) error {
	h.releaseCalled = true
	h.state = HandleReleased
	return nil
}
func (h *precheckFailHandle) State() HandleState { return h.state }

type precheckFailProvider struct{ handle *precheckFailHandle }

func (p *precheckFailProvider) Name() string { return "precheck-fail" }
func (p *precheckFailProvider) Tier() string { return TierRestricted.String() }
func (p *precheckFailProvider) State(context.Context) (ProviderState, error) {
	return ProviderPrepared, nil
}
func (p *precheckFailProvider) Capabilities(context.Context) (ProviderCapability, error) {
	return ProviderCapability{Platform: "test", AchievedIsolation: I2}, nil
}
func (p *precheckFailProvider) Prepare(context.Context, RuntimePlan) error { return nil }
func (p *precheckFailProvider) Acquire(context.Context, LeaseRequest) (RuntimeHandle, error) {
	return p.handle, nil
}

// TestRuntime_Attach_PrecheckFailure_ReleasesHandle（R-1640-1——会议 #255 P1；12 清单 G 节）：
// Precheck 失败=边界建立但未生效——句柄必须清理（07 §4.14 PrecheckFailed：销毁非归还热池；
// 销毁 vs 归还的区分=W7 热池窗口落地，当前 Release=唯一清理路径）；会话不得进 Running。
func TestRuntime_Attach_PrecheckFailure_ReleasesHandle(t *testing.T) {
	h := &precheckFailHandle{id: "h-pf", state: HandleAcquired}
	p := &precheckFailProvider{handle: h}
	s := NewExecutionSession("s-pf", "g-pf", WorkspaceRef{VolumeID: "v1", Path: "/tmp/x"})
	err := s.Attach(context.Background(), p, LeaseRequest{GoalID: "g-pf", ActionID: "a1"})
	if err == nil {
		t.Fatal("Precheck 失败应 Attach 报错")
	}
	if !h.releaseCalled {
		t.Fatal("P1：Precheck 失败路径未清理句柄（Release 未调用）——真实 Provider 持有进程/沙箱资源时=泄漏面")
	}
	if s.State() == SessionRunning {
		t.Fatal("Precheck 失败会话不得进 Running")
	}
}
