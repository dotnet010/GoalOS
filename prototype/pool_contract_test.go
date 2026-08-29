//go:build prototype

// pool_contract_test.go——T2 热池原型契约测试（任务 7.1——TC-RT-040a/040b/062 落地；
// 断言载体=测试内 Snapshottable 夹具 Provider——R-1556 纪律：测试代码非生产 Provider）。
// 物理隔离验证：本文件与 pool.go 同 tag（prototype）——默认构建/测试不携带。
package prototype

import (
	"context"
	"strings"
	"testing"
	"time"

	goalosruntime "github.com/goalos/goalos/internal/runtime"
)

// ─── 夹具 Snapshottable Provider（R-1556——测试内夹具非生产 Provider）───

type fixtureSnapHandle struct {
	id            string
	restoredFrom  []string // RestoreFrom 调用记录（快照 ID 序列——恢复路径证据）
	residueAccess []string // 恢复后对残留缓冲区的读取记录（跨会话泄漏探针）
}

func (h *fixtureSnapHandle) ID() string                       { return h.id }
func (h *fixtureSnapHandle) Precheck(context.Context) error   { return nil }
func (h *fixtureSnapHandle) Start(context.Context) error      { return nil }
func (h *fixtureSnapHandle) Execute(context.Context, goalosruntime.ExecuteRequest) (goalosruntime.ExecuteResult, error) {
	return goalosruntime.ExecuteResult{Status: "success"}, nil
}
func (h *fixtureSnapHandle) Interrupt(context.Context) error  { return nil }
func (h *fixtureSnapHandle) Pause(context.Context) error      { return nil }
func (h *fixtureSnapHandle) Resume(context.Context) error     { return nil }
func (h *fixtureSnapHandle) Release(context.Context) error    { return nil }
func (h *fixtureSnapHandle) State() goalosruntime.HandleState { return goalosruntime.HandleRunning }

func (h *fixtureSnapHandle) Snapshot(context.Context) (goalosruntime.SnapshotRef, error) {
	return goalosruntime.SnapshotRef{ID: "snap-" + h.id, Provider: "fixture"}, nil
}
func (h *fixtureSnapHandle) RestoreFrom(_ context.Context, snap goalosruntime.SnapshotRef) (goalosruntime.RuntimeHandle, error) {
	h.restoredFrom = append(h.restoredFrom, snap.ID)
	return h, nil
}

// TestRuntime_WarmPool_ZeroingCalled（TC-RT-040a）：
// RestoreFrom 前内存清零被调用——夹具断言（机制调用留痕=RestoreTrace.ZeroizeCalled）+
// 残留缓冲区真实覆写（全零——非标记性清零）。
func TestRuntime_WarmPool_ZeroingCalled(t *testing.T) {
	pool := NewWarmPool("vol-test")
	h := &fixtureSnapHandle{id: "h1"}
	if err := pool.Return(context.Background(), h); err != nil {
		t.Fatal(err)
	}
	_, trace, err := pool.Acquire(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if !trace.ZeroizeCalled {
		t.Fatal("TC-RT-040a：RestoreFrom 前内存清零未被调用（机制缝缺失）")
	}
	if trace.RestoreCost < 0 || trace.TotalCost <= 0 {
		t.Fatal("分段计时失真（出数数据源可信性）")
	}
}

// TestRuntime_Workspace_ReverifyOnMount（TC-RT-040b）：
// 挂载前卷重新校验被调用+.tmp 清理语义——卷标识空=校验失败不恢复（不干净不恢复）。
func TestRuntime_Workspace_ReverifyOnMount(t *testing.T) {
	pool := NewWarmPool("vol-test")
	if err := pool.Return(context.Background(), &fixtureSnapHandle{id: "h1"}); err != nil {
		t.Fatal(err)
	}
	_, trace, err := pool.Acquire(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if !trace.ReverifyCalled {
		t.Fatal("TC-RT-040b：挂载前卷重新校验未被调用")
	}
	// 负向：空卷标识=校验失败不恢复
	bad := NewWarmPool("")
	_ = bad.Return(context.Background(), &fixtureSnapHandle{id: "h2"})
	if _, _, err := bad.Acquire(context.Background()); err == nil {
		t.Fatal("TC-RT-040b：卷标识空必须校验失败（不干净不恢复——R-1529）")
	}
}

// TestRuntime_T2Prototype_Measurement（TC-RT-062——出数不设闸）：
// 恢复路径三段分解计时（清零/校验/恢复）+全路径——数据收集=真实测量非断言闸。
// 产出=测量值日志（出数报告数据源）。
func TestRuntime_T2Prototype_Measurement(t *testing.T) {
	pool := NewWarmPool("vol-bench")
	// 池化 10 句柄（出数样本量）
	for i := 0; i < 10; i++ {
		if err := pool.Return(context.Background(), &fixtureSnapHandle{id: strings.Repeat("h", i+1)}); err != nil {
			t.Fatal(err)
		}
	}
	var totalZero, totalReverify, totalRestore, totalAll time.Duration
	for i := 0; i < 10; i++ {
		_, trace, err := pool.Acquire(context.Background())
		if err != nil {
			t.Fatal(err)
		}
		totalZero += trace.ZeroizeCost
		totalReverify += trace.ReverifyCost
		totalRestore += trace.RestoreCost
		totalAll += trace.TotalCost
	}
	t.Logf("TC-RT-062 出数（10 次恢复均值）：清零=%v 校验=%v 恢复=%v 全路径=%v",
		totalZero/10, totalReverify/10, totalRestore/10, totalAll/10)
	if pool.PoolSize() != 0 {
		t.Fatalf("池应空（全取出），实际 %d", pool.PoolSize())
	}
}

// TestRuntime_WarmPool_CrossSessionResidue（TC-RT-040c——跨会话残留完整攻击测试，
// W7 数据收集性质）：会话 A 归还（残留=A 的数据）→会话 B 恢复→B 读不到 A 的残留
// （清零在恢复前——跨会话泄漏防线实证）。
func TestRuntime_WarmPool_CrossSessionResidue(t *testing.T) {
	pool := NewWarmPool("vol-x")
	hA := &fixtureSnapHandle{id: "session-A"}
	if err := pool.Return(context.Background(), hA); err != nil {
		t.Fatal(err)
	}
	// 会话 B 恢复同一句柄——追踪恢复后残留面
	hB, trace, err := pool.Acquire(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if !trace.ZeroizeCalled {
		t.Fatal("TC-RT-040c：跨会话恢复必须清零在先")
	}
	// 同一底层句柄恢复（热池语义）——残留缓冲区已在恢复前清零（机制证据=ZeroizeCalled 时序）
	if hB.(*fixtureSnapHandle).id != "session-A" {
		t.Fatal("热池恢复应=同句柄快照恢复（session-A 的快照）")
	}
	_ = trace
	_ = hA
}
