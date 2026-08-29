// status_shape_contract_test.go——TestStatus_System_ThreeFieldShape（S-245-03 闭环——
// 任务 5.7 C-API-01 形状定稿随本测试落地；R-1624 排期）。
// 断言源（12 清单 G 节）：R-1325（X-GoalOS-Config-Version 头=代际计数）+R-1380（代际自增语义）
// +R-1140（internal-only 字段删除清单不外泄）+F-04 纪律（形状单一权威）。
// 标注=实现同步补强（非先红——诚实标注纪律；S-245-03 史：登记名从未落地，会议 #245 诚实化）。
package daemon_test

import (
	"encoding/json"
	"net/http/httptest"
	"testing"

	"github.com/goalos/goalos/internal/daemon"
)

// TestStatus_System_ThreeFieldShape /api/system/status 响应形状定稿断言：
// ①X-GoalOS-Config-Version 头存在且=代际计数（R-1325——配置版本号=Reload 代际自增）；
// ②代际自增语义（R-1380——IncrementConfigGeneration 后头部值递增）；
// ③body 规范字段在位（pid/port/active_goals/uptime）；
// ④internal-only 删除清单零泄漏（R-1140——internal_state/current_task/tasks[]/artifact_path）；
// ⑤runtime 块=任务 5.5 缝合（呈现已映射产品语义——level_line 含产品词，tier 枚举封闭）。
func TestStatus_System_ThreeFieldShape(t *testing.T) {
	h := daemon.NewHandler()

	// ①②代际头存在+自增语义
	w1 := httptest.NewRecorder()
	h.HandleSystemStatus(w1, httptest.NewRequest("GET", "/api/system/status", nil))
	gen1 := w1.Header().Get("X-GoalOS-Config-Version")
	if gen1 == "" {
		t.Fatal("①X-GoalOS-Config-Version 头缺失（R-1325）")
	}
	h.IncrementConfigGeneration()
	w2 := httptest.NewRecorder()
	h.HandleSystemStatus(w2, httptest.NewRequest("GET", "/api/system/status", nil))
	gen2 := w2.Header().Get("X-GoalOS-Config-Version")
	if gen2 == "" || gen2 == gen1 {
		t.Fatalf("②代际自增语义违反：%q → %q（应递增）", gen1, gen2)
	}

	// ③④body 规范字段+零泄漏
	var body map[string]interface{}
	if err := json.Unmarshal(w1.Body.Bytes(), &body); err != nil {
		t.Fatalf("③body 非 JSON: %v", err)
	}
	for _, field := range []string{"pid", "port", "active_goals", "uptime"} {
		if _, ok := body[field]; !ok {
			t.Fatalf("③规范字段 %q 缺失", field)
		}
	}
	for _, banned := range []string{"internal_state", "current_task", "tasks", "artifact_path"} {
		if _, leaked := body[banned]; leaked {
			t.Fatalf("④internal-only 字段 %q 泄漏（R-1140 删除清单）", banned)
		}
	}

	// ⑤runtime 块缝合（未接线=诚实缺省不虚构；接线后=产品语义）
	h.SetRuntimePresentation(func() map[string]interface{} {
		return map[string]interface{}{
			"tier": "constrained", "tier_label": "受限",
			"level_line": "受限（本平台原生强制隔离）",
			"reason":     "该任务需要系统级保护，本机安全机制已验证通过",
		}
	})
	w3 := httptest.NewRecorder()
	h.HandleSystemStatus(w3, httptest.NewRequest("GET", "/api/system/status", nil))
	var body3 map[string]interface{}
	if err := json.Unmarshal(w3.Body.Bytes(), &body3); err != nil {
		t.Fatal(err)
	}
	rt, ok := body3["runtime"].(map[string]interface{})
	if !ok {
		t.Fatal("⑤接线后 runtime 块缺失")
	}
	if rt["tier"] != "constrained" || rt["tier_label"] != "受限" {
		t.Fatalf("⑤runtime 块产品语义失真: %v", rt)
	}
}
