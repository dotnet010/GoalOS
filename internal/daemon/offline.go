// Package daemon — 离线降级处理。
// LLM API 不可用→Goal 标记为 planned（待联网执行）。
// 本地操作（文件读写/Shell）在离线时正常工作。
//
// 设计依据：05 架构文档 §8、R18。
package daemon

import (
	"net/http"
	"time"
)

// CheckOnline 检查是否能访问互联网。
// 使用 HEAD 请求到 google.com（快速、轻量）。
func CheckOnline() bool {
	client := &http.Client{Timeout: 3 * time.Second}
	resp, err := client.Head("https://google.com")
	if err != nil {
		return false
	}
	resp.Body.Close()
	return resp.StatusCode < 500
}

// OfflineStatus 是离线状态追踪器。
type OfflineStatus struct {
	IsOnline   bool
	LastCheck  time.Time
	PendingGoals []string // 离线时创建的 Goal——联网后自动执行
}

// NewOfflineStatus 创建离线状态追踪器。
func NewOfflineStatus() *OfflineStatus {
	return &OfflineStatus{
		IsOnline: CheckOnline(),
	}
}

// Refresh 刷新在线状态。
func (os *OfflineStatus) Refresh() {
	os.IsOnline = CheckOnline()
	os.LastCheck = time.Now()
}

// QueueOfflineGoal 记录离线时创建的 Goal。
func (os *OfflineStatus) QueueOfflineGoal(goalID string) {
	os.PendingGoals = append(os.PendingGoals, goalID)
}

// PopPendingGoals 取出所有待处理的 Goal（联网后执行）。
func (os *OfflineStatus) PopPendingGoals() []string {
	goals := os.PendingGoals
	os.PendingGoals = nil
	return goals
}
