// session_contract_test.go——任务 3.3 契约测试（R-571 先红；TC-RT-070 升级端到端三断言：
// 取消时序/Cancelled 标记/重执行不重复产出——规格=05 §X.6.6+R-1499/R-1504/R-1509/
// R-1515/R-1522/R-1525/R-1529；升级=优雅取消+新会话挂同卷，粒度=Action 1:1）。
package runtime

import (
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
	interruptSeq  []string // Interrupt 调用时序记录（取消协议链断言数据源）
	executed      []string // 已执行 Action 序列
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
	h.executed = append(h.executed, req.ActionID)
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
// T0→受限档升级端到端——①取消时序（Interrupt 链被调用一次）②被中断 Action 标记 Cancelled
// ③新会话挂同卷重执行该 Action（已完成 Action 产出物不重复、审计事件链完整——R-1499）。
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
	sess.MarkActionExecuting("act-2")

	newSess, err := sess.Escalate(ctx, providerT1, "capability_proxy")
	if err != nil {
		t.Fatalf("升级失败: %v", err)
	}

	// 断言①取消时序：Interrupt 被调用恰好一次（优雅取消协议链入口）
	if len(handle.interruptSeq) != 1 {
		t.Fatalf("断言①失败：Interrupt 应调用 1 次，实际 %d", len(handle.interruptSeq))
	}
	// 断言②被中断 Action 标记 Cancelled
	if got := sess.ActionStatus("act-2"); got != "Cancelled" {
		t.Fatalf("断言②失败：act-2 应=Cancelled，实际 %q", got)
	}
	// 断言③新会话挂同卷+重执行 act-2+已完成 act-1 不重复产出
	if newSess.Workspace().VolumeID != "vol-1" {
		t.Fatalf("断言③失败：新会话卷=%q 应=vol-1（同卷引用不变）", newSess.Workspace().VolumeID)
	}
	if err := newSess.ReexecuteInterrupted(ctx); err != nil {
		t.Fatalf("重执行失败: %v", err)
	}
	// act-1 不在新会话重执行（已完成产出物不重复）
	for _, id := range handleT1.executed {
		if id == "act-1" {
			t.Fatal("断言③失败：已完成 act-1 在新会话被重复执行——产出物重复")
		}
	}
	found := false
	for _, id := range handleT1.executed {
		if id == "act-2" {
			found = true
		}
	}
	if !found {
		t.Fatal("断言③失败：act-2 未在新会话重执行")
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
	sess.MarkActionExecuting("act-1")

	_, err := sess.Escalate(ctx, &upgradeFakeProvider{name: "t1", tier: "T1", handle: &upgradeFakeHandle{id: "h-y", state: HandleAcquired}}, "risk_reeval")
	if err == nil {
		t.Fatal("Interrupt 失败的升级必须失败（重签/取消失败=任务失败）")
	}
	// 旧会话清理失败=强制销毁——状态=已销毁
	if sess.State() != SessionDestroyed {
		t.Fatalf("Interrupt 失败后旧会话应=强制销毁，实际: %v", sess.State())
	}
}
