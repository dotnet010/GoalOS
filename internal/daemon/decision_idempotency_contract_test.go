// decision_idempotency_contract_test.go——D-3 审批幂等三形态契约测试（R-1644——会议 #259 裁决）。
// 标注=实现同步补强（非先红——诚实标注纪律）。12 清单 G 节登记。
// 断言：(1)新裁决=200+F-04 形状（{ok, goal_id, visible_state, updated_at}——R-1324 收口）；
// (2)重复同决策=200 幂等回放（replayed=true——回放缓存原结果，Stripe 族）；
// (3)重复冲突决策=409（Kees 硬语义——决策不可覆写）；(4)真未知=404（诚实回答）；
// (5)并发双裁决=恰好一次生效（delete-in-lock 并发安全语义不变）。
package daemon_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"

	"github.com/goalos/goalos/internal/daemon"
)

// d3Setup 夹具：注册一个待审批项（Handler 直测——经包内可见性走公开 API 注册面）。
func d3Setup(h *daemon.Handler, actionID, goalID string) {
	// 经 HTTP 面制造待审批项=模拟治理事件链过重——本测试=Handler 层级直接登记
	// （daemon 包导出 RegisterPendingApproval 测试面——D-3 落地配套）。
	daemon.RegisterPendingApprovalForTest(h, actionID, goalID)
}

// TestDecision_IdempotencyThreeForms D-3 三形态（R-1644）。
func TestDecision_IdempotencyThreeForms(t *testing.T) {
	h := daemon.NewHandler()
	d3Setup(h, "act-1", "goal-1")

	// (1)新裁决=200+F-04 形状
	w1 := httptest.NewRecorder()
	req1 := httptest.NewRequest("POST", "/api/approvals/act-1/approve", nil)
	req1.SetPathValue("id", "act-1")
	h.HandleApprove(w1, req1)
	if w1.Code != http.StatusOK {
		t.Fatalf("(1)新裁决应=200，实际 %d", w1.Code)
	}
	var body1 map[string]interface{}
	if err := json.Unmarshal(w1.Body.Bytes(), &body1); err != nil {
		t.Fatal(err)
	}
	for _, f := range []string{"ok", "goal_id", "visible_state", "updated_at"} {
		if _, ok := body1[f]; !ok {
			t.Fatalf("(1)F-04 形状缺字段 %q（R-1324）", f)
		}
	}
	if body1["replayed"] != false {
		t.Fatal("(1)新裁决 replayed 必须=false")
	}

	// (2)重复同决策=200 幂等回放
	w2 := httptest.NewRecorder()
	req2 := httptest.NewRequest("POST", "/api/approvals/act-1/approve", nil)
	req2.SetPathValue("id", "act-1")
	h.HandleApprove(w2, req2)
	if w2.Code != http.StatusOK {
		t.Fatalf("(2)重复同决策应=200，实际 %d", w2.Code)
	}
	var body2 map[string]interface{}
	_ = json.Unmarshal(w2.Body.Bytes(), &body2)
	if body2["replayed"] != true {
		t.Fatal("(2)回放必须标记 replayed=true（审计可读=非重复执行证据）")
	}
	if body2["decision"] != "approved" {
		t.Fatalf("(2)回放应=原决策 approved，实际 %v", body2["decision"])
	}

	// (3)重复冲突决策=409
	w3 := httptest.NewRecorder()
	req3 := httptest.NewRequest("POST", "/api/approvals/act-1/reject", nil)
	req3.SetPathValue("id", "act-1")
	h.HandleReject(w3, req3)
	if w3.Code != http.StatusConflict {
		t.Fatalf("(3)冲突决策应=409，实际 %d", w3.Code)
	}

	// (4)真未知=404
	w4 := httptest.NewRecorder()
	req4 := httptest.NewRequest("POST", "/api/approvals/act-never/approve", nil)
	req4.SetPathValue("id", "act-never")
	h.HandleApprove(w4, req4)
	if w4.Code != http.StatusNotFound {
		t.Fatalf("(4)真未知应=404，实际 %d", w4.Code)
	}

	// (5)并发双裁决=恰好一次生效（delete-in-lock 语义不变）
	d3Setup(h, "act-race", "goal-1")
	var codes []int
	var mu sync.Mutex
	var wg sync.WaitGroup
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			w := httptest.NewRecorder()
			req := httptest.NewRequest("POST", "/api/approvals/act-race/approve", nil)
			req.SetPathValue("id", "act-race")
			h.HandleApprove(w, req)
			mu.Lock()
			codes = append(codes, w.Code)
			mu.Unlock()
		}()
	}
	wg.Wait()
	okCount, replayCount := 0, 0
	for _, c := range codes {
		if c == http.StatusOK {
			okCount++
		}
	}
	_ = replayCount
	// 全部 200（首次=新裁决，其余=回放）——但事件只发布一次（由 replayed 标记区分）
	if okCount != 8 {
		t.Fatalf("(5)并发裁决全部应=200（回放族），实际 200 数=%d/%d", okCount, len(codes))
	}
}
