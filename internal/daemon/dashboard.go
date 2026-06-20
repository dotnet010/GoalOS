// Package daemon — Goal 页面 HTML 仪表盘。
// 单页应用。WebSocket 实时更新。Goal 输入 + 进度展示 + Watcher 审批。
// 设计依据：02 低保真原型、01 PRD §3。
package daemon

import (
	_ "embed"
	"net/http"
)

//go:embed dashboard.html
var dashboardHTML string

// HandleDashboard 提供 Goal 页面。GET /。
func HandleDashboard(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Write([]byte(dashboardHTML))
}
