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
	// G14: 多端点检测——Google/Baidu/GitHub 任一可达即为在线
	endpoints := []string{
		"https://google.com",
		"https://baidu.com",
		"https://github.com",
	}
	client := &http.Client{Timeout: 3 * time.Second}
	for _, url := range endpoints {
		resp, err := client.Head(url)
		if err == nil {
			resp.Body.Close()
			if resp.StatusCode < 500 {
				return true
			}
		}
	}
	return false
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
