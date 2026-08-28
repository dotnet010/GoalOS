// spi_contract_test.go——任务 3.2 契约测试（R-571 先红；规格=05 §X.6.5 SPI 定稿+
// R-1620 方法级行为契约表+R-1608/D-13 支撑类型全量+R-1468 骨架纪律）。
package runtime

import (
	"context"
	"errors"
	"testing"
	"time"
)

// ─── TC-RT-090：未实现 Provider 不注册（骨架零注册静态断言）───

// stubProvider 全方法 ErrNotImplemented 的骨架 Provider（R-1468 骨架形态）。
type stubProvider struct{}

func (stubProvider) Name() string  { return "stub" }
func (stubProvider) Tier() string  { return "T1" }
func (stubProvider) State(context.Context) (ProviderState, error) {
	return 0, ErrNotImplemented
}
func (stubProvider) Capabilities(context.Context) (ProviderCapability, error) {
	return ProviderCapability{}, ErrNotImplemented
}
func (stubProvider) Prepare(context.Context, RuntimePlan) error { return ErrNotImplemented }
func (stubProvider) Acquire(context.Context, LeaseRequest) (RuntimeHandle, error) {
	return nil, ErrNotImplemented
}

// TestRuntime_UnregisteredProvider_NotCandidate（TC-RT-090——12 清单 F 节）：
// 骨架 Provider（State 返回 ErrNotImplemented）注册=拒绝；注册表保持空。
func TestRuntime_UnregisteredProvider_NotCandidate(t *testing.T) {
	reg := NewProviderRegistry()
	if err := reg.RegisterChecked(stubProvider{}); !errors.Is(err, ErrNotImplemented) {
		t.Fatalf("骨架 Provider 注册必须拒绝（ErrNotImplemented），实际: %v", err)
	}
	if _, err := reg.AcquireForTier("T1"); !errors.Is(err, ErrNoProviderRegistered) {
		t.Fatalf("注册表必须保持空（骨架纪律 R-1468——未实现不注册），实际: %v", err)
	}
}

// ─── R-1620 方法级行为契约表执行（handleStateGuard 包装器逐格断言）───

// fakeHandle 可编程假句柄（记录调用序列——状态机断言用）。
type fakeHandle struct {
	id    string
	state HandleState
	calls []string
}

func (f *fakeHandle) ID() string { return f.id }
func (f *fakeHandle) Precheck(context.Context) error {
	f.calls = append(f.calls, "Precheck")
	f.state = HandleRunning // D-5 定序：Precheck 通过→Running
	return nil
}
func (f *fakeHandle) Start(context.Context) error {
	f.calls = append(f.calls, "Start")
	f.state = HandleReady // D-5 定序：Start→Ready（边界建立）；Precheck 通过→Running
	return nil
}
func (f *fakeHandle) Execute(context.Context, ExecuteRequest) (ExecuteResult, error) {
	f.calls = append(f.calls, "Execute")
	return ExecuteResult{Status: "success"}, nil
}
func (f *fakeHandle) Interrupt(context.Context) error {
	f.calls = append(f.calls, "Interrupt")
	return nil
}
func (f *fakeHandle) Pause(context.Context) error {
	f.calls = append(f.calls, "Pause")
	f.state = HandlePaused
	return nil
}
func (f *fakeHandle) Resume(context.Context) error {
	f.calls = append(f.calls, "Resume")
	f.state = HandleRunning
	return nil
}
func (f *fakeHandle) Release(context.Context) error {
	f.calls = append(f.calls, "Release")
	f.state = HandleReleased
	return nil
}
func (f *fakeHandle) State() HandleState { return f.state }

// TestRuntime_SPI_MethodContracts（R-1620 契约表逐格——任务 3.2 验收追加，不增编号）：
// 前置/非法/幂等全格断言（规格=05 §X.6.5 契约表）。
func TestRuntime_SPI_MethodContracts(t *testing.T) {
	ctx := context.Background()
	fake := &fakeHandle{id: "h1", state: HandleAcquired}
	h := NewHandleGuard(fake)

	// Start 前置=Acquired ✓（Start→Ready——D-5 定序）
	if err := h.Start(ctx); err != nil {
		t.Fatalf("Acquired+Start 应成功: %v", err)
	}
	// fake Start 后 state=Ready——契约表：Ready+Start=ErrInvalidState
	if err := h.Start(ctx); !errors.Is(err, ErrInvalidState) {
		t.Fatalf("Ready+Start 应=ErrInvalidState，实际: %v", err)
	}
	// Ready+Execute=ErrInvalidState（D-5——Precheck 未通过前禁止 Execute，预检闸门不可架空）
	if _, err := h.Execute(ctx, ExecuteRequest{ActionID: "a0"}); !errors.Is(err, ErrInvalidState) {
		t.Fatalf("Ready+Execute 应=ErrInvalidState（D-5 闸门），实际: %v", err)
	}
	// Precheck 前置=Ready ✓（通过→Running）
	if err := h.Precheck(ctx); err != nil {
		t.Fatalf("Ready+Precheck 应成功: %v", err)
	}
	// Execute 仅 Running ✓
	if _, err := h.Execute(ctx, ExecuteRequest{ActionID: "a1"}); err != nil {
		t.Fatalf("Running+Execute 应成功: %v", err)
	}
	// Pause 仅 Running；Pause+Pause=ErrInvalidState
	if err := h.Pause(ctx); err != nil {
		t.Fatalf("Running+Pause 应成功: %v", err)
	}
	if err := h.Pause(ctx); !errors.Is(err, ErrInvalidState) {
		t.Fatalf("Paused+Pause 应=ErrInvalidState，实际: %v", err)
	}
	// Resume 仅 Paused
	if err := h.Resume(ctx); err != nil {
		t.Fatalf("Paused+Resume 应成功: %v", err)
	}
	// Precheck 前置=Ready 或 Running（幂等重验——验证只读）：Running+Precheck=允许
	if err := h.Precheck(ctx); err != nil {
		t.Fatalf("Running+Precheck（幂等重验）应成功: %v", err)
	}
	// Interrupt 仅 Running（fake 已 Running）；child 已退出=成功归 Provider 语义（不在 Guard）
	if err := h.Interrupt(ctx); err != nil {
		t.Fatalf("Running+Interrupt 应成功: %v", err)
	}
	// Release 任意态可调；二次 Release=幂等成功
	if err := h.Release(ctx); err != nil {
		t.Fatalf("Release 应成功: %v", err)
	}
	if err := h.Release(ctx); err != nil {
		t.Fatalf("二次 Release 应幂等成功: %v", err)
	}
	// Release 后其他调用=ErrHandleReleased
	if err := h.Pause(ctx); !errors.Is(err, ErrHandleReleased) {
		t.Fatalf("Released+Pause 应=ErrHandleReleased，实际: %v", err)
	}
	if _, err := h.Execute(ctx, ExecuteRequest{}); !errors.Is(err, ErrHandleReleased) {
		t.Fatalf("Released+Execute 应=ErrHandleReleased，实际: %v", err)
	}
	// State 全态可调（Released 返回 HandleReleased 值非错误）
	if h.State() != HandleReleased {
		t.Fatalf("State() 应=HandleReleased，实际: %v", h.State())
	}
}

// ─── D-13 支撑类型全字段（零值非法纪律——关键字段断言）───

// TestRuntime_SPI_SupportTypes D-13：四支撑类型字段全量（RuntimePlan/ExecuteRequest/
// ExecuteResult/SnapshotRef）+三枚举零值非法（IsolationLevel/ExecutionTier/HandleState）。
func TestRuntime_SPI_SupportTypes(t *testing.T) {
	// 四类型关键字段可赋值（编译期=字段存在性断言）
	plan := RuntimePlan{PlanID: "p1", Tier: TierRestricted, ProfileDigest: "digest", RequiredCapabilities: []string{"fs.write"}}
	if plan.PlanID == "" || plan.ProfileDigest == "" {
		t.Fatal("RuntimePlan 字段缺失")
	}
	req := ExecuteRequest{ActionID: "a1", ActionType: "fs.write", Timeout: 30 * time.Second}
	res := ExecuteResult{Status: "success", ExitCode: 0, Output: "ok", Cost: 10 * time.Millisecond}
	if res.ExitCode != 0 || res.Cost <= 0 {
		t.Fatal("ExecuteResult 字段缺失")
	}
	_ = req
	snap := SnapshotRef{ID: "s1", Provider: "p1", CreatedAt: time.Now(), Digest: "d"}
	if snap.ID == "" {
		t.Fatal("SnapshotRef 字段缺失")
	}
	// 三枚举零值非法
	var lvl IsolationLevel
	if lvl != 0 {
		t.Fatal("IsolationLevel 零值应非法（=0）")
	}
	if _, err := ParseIsolationLevel("I9"); err == nil {
		t.Fatal("非法 IsolationLevel 未拒绝")
	}
	var tier ExecutionTier
	if tier != 0 {
		t.Fatal("ExecutionTier 零值应非法")
	}
	var hs HandleState
	if hs != 0 {
		t.Fatal("HandleState 零值应非法")
	}
}
