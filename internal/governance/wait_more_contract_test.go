// wait_more_contract_test.go——D-4 审批延期契约测试（R-1645——会议 #260 裁决）。
// 标注=实现同步补强（非先红——诚实标注纪律）。12 清单 G 节登记。
// 断言：①延期接受+计时器重启（Temporal snooze——取消旧+新完整窗口）；②extensions_used 计数；
// ③上限=3 次第 4 次=GOV-APX-F-001 即时返回（计时器继续——上限≠立即拒绝——R-1603 逐字）；
// ④已裁决项 wait_more=不接受（幂等无副作用）；⑤时序实证：延期后超时点在延后期满（非原窗口）。
// 时序纪律：秒级窗口（本机调度延迟秒级——百毫秒窗口随机红教训）。
package governance

import (
	"testing"
	"time"

	"github.com/goalos/goalos/internal/eventbus"
	"github.com/goalos/goalos/pkg/events"
)

// waitMoreFixture 构造一个进行中的审批（包内直构造——timer 字段同包可见）。
func waitMoreFixture(e *Engine, actionID string, timeoutSec float64) {
	e.pendingMu.Lock()
	defer e.pendingMu.Unlock()
	e.pendingApprovals[actionID] = pendingApproval{
		goalID: "g-wm", actionID: actionID, actionType: "test.action",
		timeoutSec: timeoutSec,
		timer: time.AfterFunc(time.Duration(timeoutSec)*time.Second, func() {
			e.handleApprovalTimeout("g-wm", actionID, Decision{})
		}),
	}
}

// TestGovernance_WaitMore_Extension D-4 五断言（R-1645/R-1603）。
func TestGovernance_WaitMore_Extension(t *testing.T) {
	bus := eventbus.New()
	e := New(bus, nil)
	e.Start()

	// ①延期接受+计数
	waitMoreFixture(e, "act-wm", 300) // 长窗口——本断言不测时序
	accepted, used, exhausted := e.WaitMore("act-wm")
	if !accepted || used != 1 || exhausted {
		t.Fatalf("①首次延期应=接受+used=1+未超限，实际: accepted=%v used=%d exhausted=%v", accepted, used, exhausted)
	}

	// ③上限：第 2/3 次接受，第 4 次=超限即时返回（计时器继续——R-1603）
	e.WaitMore("act-wm")
	e.WaitMore("act-wm")
	accepted4, used4, exhausted4 := e.WaitMore("act-wm")
	if accepted4 || !exhausted4 || used4 != 3 {
		t.Fatalf("③第 4 次应=超限即时返回（accepted=false exhausted=true used=3），实际: %v/%v/%d", accepted4, exhausted4, used4)
	}

	// ④已裁决/未知项=不接受（幂等无副作用——D-3 三形态同族）
	acceptedX, _, _ := e.WaitMore("act-never")
	if acceptedX {
		t.Fatal("④未知审批项 wait_more 必须不接受")
	}

	// ⑤时序实证：延期后超时点=延后期满（非原窗口）——秒级窗口纪律
	bus2 := eventbus.New()
	e2 := New(bus2, nil)
	e2.Start()
	rejected := make(chan struct{}, 1)
	bus2.Subscribe(events.TypeActionRejected, func(evt events.Event) error {
		rejected <- struct{}{}
		return nil
	})
	waitMoreFixture(e2, "act-timing", 2) // 原窗口=2s
	time.Sleep(1200 * time.Millisecond) // t≈1.2s：延期（新窗口=2s 自此——期满≈3.2s）
	if ok, _, _ := e2.WaitMore("act-timing"); !ok {
		t.Fatal("⑤延期应接受")
	}
	select {
	case <-rejected:
		t.Fatal("⑤原窗口期内（<3.2s）不应拒绝——延期未生效")
	case <-time.After(1500 * time.Millisecond): // t≈2.7s——原窗口（2s）已过=未拒绝=延期生效证据
	}
	select {
	case <-rejected:
		// 延后期满后拒绝=正确（t≈3.2s——原窗口+延期窗口）
	case <-time.After(2500 * time.Millisecond):
		t.Fatal("⑤延长期满（≈3.2s）后必须超时兜底拒绝（审批必须终结——R-1603）")
	}
}
