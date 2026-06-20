// Daemon 集成测试——验证离线降级 + OS 通知。
// 设计依据：R238（每模块错误路径）、R239（API模块≥3+1集成）。
package daemon_test

import (
	"testing"

	"github.com/goalos/goalos/internal/daemon"
)

// TestOfflineQueue 验证离线 Goal 队列机制。
// 行为：离线时创建 Goal→加入队列→联网后取出。
func TestOfflineQueue(t *testing.T) {
	os := daemon.NewOfflineStatus()

	// 模拟离线
	os.IsOnline = false
	os.QueueOfflineGoal("goal_offline_001")
	os.QueueOfflineGoal("goal_offline_002")

	if len(os.PendingGoals) != 2 {
		t.Errorf("expected 2 pending goals, got %d", len(os.PendingGoals))
	}

	// 模拟联网→取出待处理
	goals := os.PopPendingGoals()
	if len(goals) != 2 {
		t.Errorf("expected 2 popped goals, got %d", len(goals))
	}
	if len(os.PendingGoals) != 0 {
		t.Errorf("queue should be empty after pop, got %d", len(os.PendingGoals))
	}
}

// TestOfflineRefresh 验证在线状态刷新。
func TestOfflineRefresh(t *testing.T) {
	os := daemon.NewOfflineStatus()
	os.Refresh()
	// 无法保证网络状态——仅验证不 panic
	if os.LastCheck.IsZero() {
		t.Error("LastCheck should be set after Refresh")
	}
}

// TestNotifyError 验证通知在非 macOS/Linux 平台不 panic。
func TestNotifyError(t *testing.T) {
	// 通知失败不应 panic
	daemon.NotifyGoalCompleted("测试目标", "/tmp/test")
	// 如果走到这里没有 panic——测试通过
}
