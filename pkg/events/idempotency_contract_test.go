// idempotency_contract_test.go — 幂等键形态契约测试（R-1373）。
//
// 断言来源: R-1373「幂等键={goal_id}-{action_id}-{session_id}」——重试幂等
// 由 {goal, action, session} 三元组承载，键不得含 seq。
//
// 先红状态（当前为何红）: 当前 WithAction 生成 ID={goalID}-{actionID}-{seq}
// （含 seq）——同 {goal, action, session} 不同 seq 的重试事件会得到不同幂等键，
// 线格式键形态断言失败 → 测试红。
//
// 转绿任务: 1.39/3.19（WithAction 幂等键去 seq、改拼 session_id）。
package events

import (
	"encoding/json"
	"fmt"
	"strings"
	"testing"
)

// TestIdempotencyKey_StableAcrossRetry —— R-1373: 同 {goal, action, session}
// 不同 seq 的重试事件必须序列化为相同 idempotency_key，且键形态
// = {goal_id}-{action_id}-{session_id}。
func TestIdempotencyKey_StableAcrossRetry(t *testing.T) {
	const (
		goalID    = "goal-47"
		actionID  = "act-exec-3"
		sessionID = "session-7"
	)
	mk := func(seq int) map[string]any {
		evt := NewEvent(TypeActionStarted, goalID, "plugin_runner")
		evt.Seq = seq
		evt.SessionID = sessionID
		evt = evt.WithAction(actionID) // 幂等键生成点——必须只依赖 {goal, action, session}
		data, err := json.Marshal(evt)
		if err != nil {
			t.Fatalf("前置: json.Marshal(seq=%d): %v", seq, err)
		}
		var m map[string]any
		if err := json.Unmarshal(data, &m); err != nil {
			t.Fatalf("前置: json.Unmarshal: %v", err)
		}
		return m
	}
	m1 := mk(101)
	m2 := mk(205) // 重试后的新 seq
	want := fmt.Sprintf("%s-%s-%s", goalID, actionID, sessionID)

	// MUST 1: 线格式幂等键 = {goal_id}-{action_id}-{session_id}（R-1373）。
	if got := m1["idempotency_key"]; got != want {
		t.Errorf("R-1373 MUST 1 FAIL: idempotency_key=%v，期望 %q（当前实现含 seq——重试会生成不同键）", got, want)
	}
	// MUST 2: 同三元组不同 seq → 键必须稳定不变（重试幂等）。
	if k1, k2 := m1["idempotency_key"], m2["idempotency_key"]; k1 != k2 {
		t.Errorf("R-1373 MUST 2 FAIL: 同 {goal,action,session} 不同 seq 键不同：%v vs %v——键不得含 seq", k1, k2)
	}
	// MUST 3: 键不得包含 seq 数值（防 seq 掺入键形态）。
	if s, ok := m1["idempotency_key"].(string); ok && strings.Contains(s, "101") {
		t.Errorf("R-1373 MUST 3 FAIL: idempotency_key=%q 含 seq——键形态错误", s)
	}
}
