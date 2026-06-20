// Daemon API 全端点集成测试——验证所有 12 个 HTTP 端点。
// 设计依据：05 架构文档 §2.2、R194。
package daemon_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/goalos/goalos/internal/daemon"
)

// TestAllAPIEndpoints 验证全部 12 个 API 端点可访问且返回正确状态码。
// 行为：启动 daemon.Handler→逐个请求所有端点→验证 HTTP 状态码。
func TestAllAPIEndpoints(t *testing.T) {
	h := daemon.NewHandler()

	// 先创建 Goal 以便后续操作
	body := strings.NewReader(`{"goal":"API全端点测试"}`)
	req := httptest.NewRequest("POST", "/api/goals", body)
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	h.HandleCreateGoal(w, req)
	var resp map[string]interface{}
	json.NewDecoder(w.Body).Decode(&resp)
	goalID := resp["goal_id"].(string)

	tests := []struct {
		name   string
		method string
		path   string
		body   *strings.Reader
		want   int
	}{
		{"健康检查", "GET", "/api/health", nil, 200},
		{"创建Goal", "POST", "/api/goals", strings.NewReader(`{"goal":"test"}`), 201},
		{"查询Goal", "GET", "/api/goals/" + goalID, nil, 200},
		{"列出Goal", "GET", "/api/goals", nil, 200},
		{"暂停", "POST", "/api/goals/" + goalID + "/pause", nil, 200},
		{"恢复", "POST", "/api/goals/" + goalID + "/resume", nil, 200},
		{"终止", "POST", "/api/goals/" + goalID + "/stop", nil, 200},
		{"审计日志", "GET", "/api/goals/" + goalID + "/log", nil, 200},
		{"事件导出", "GET", "/api/goals/" + goalID + "/events", nil, 200},
		{"系统状态", "GET", "/api/system/status", nil, 200},
		{"重启", "POST", "/api/system/restart", nil, 202},
		{"停止", "POST", "/api/system/stop", nil, 202},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var req *http.Request
			if tt.body != nil {
				req = httptest.NewRequest(tt.method, tt.path, tt.body)
				req.Header.Set("Content-Type", "application/json")
			} else {
				req = httptest.NewRequest(tt.method, tt.path, nil)
			}
			w := httptest.NewRecorder()
			// 为需要 :id 的端点设置 PathValue
			if strings.Contains(tt.path, goalID) {
				req.SetPathValue("id", goalID)
			}
			switch {
			case tt.path == "/api/system/stop":
				h.HandleDaemonStop(w, req)
			case tt.path == "/api/system/restart":
				h.HandleDaemonRestart(w, req)
			case tt.path == "/api/system/status":
				h.HandleSystemStatus(w, req)
			case tt.path == "/api/health":
				h.HandleHealth(w, req)
			case tt.path == "/api/goals" && tt.method == "POST":
				h.HandleCreateGoal(w, req)
			case tt.path == "/api/goals" && tt.method == "GET":
				h.HandleListGoals(w, req)
			case strings.HasSuffix(tt.path, "/pause"):
				h.HandlePauseGoal(w, req)
			case strings.HasSuffix(tt.path, "/resume"):
				h.HandleResumeGoal(w, req)
			case strings.HasSuffix(tt.path, "/stop"):
				h.HandleStopGoal(w, req)
			case strings.HasSuffix(tt.path, "/log"):
				h.HandleGoalLog(w, req)
			case strings.HasSuffix(tt.path, "/events"):
				h.HandleGoalEvents(w, req)
			default:
				h.HandleGetGoal(w, req)
			}

			if w.Code != tt.want {
				t.Errorf("%s: expected %d, got %d (%s)", tt.name, tt.want, w.Code, w.Body.String())
			}
		})
	}
}
