// Package daemon — v0.2.2 W6 CR 内部测试: failHints 查表 + SetGoalErrorHint
package daemon

import (
	"testing"
)

// TestFailHints_Lookup_ExactMatch 验证 failHints 精确匹配（W6-T2）。
func TestFailHints_Lookup_ExactMatch(t *testing.T) {
	tests := []struct {
		hint     string
		wantKey  string
	}{
		{"execution_error", "execution_error"},
		{"execution_error: connection refused", "execution_error"},
		{"execution_error timeout", "execution_error"},
		{"llm_timeout", "llm_timeout"},
		{"llm_timeout: request deadline exceeded", "llm_timeout"},
		{"no_output", "no_output"},
		{"plugin_not_found: shell-executor", "plugin_not_found"},
	}
	for _, tt := range tests {
		found := false
		for key := range failHints {
			if tt.hint == key || len(tt.hint) > len(key) && (tt.hint[len(key)] == ':' || tt.hint[len(key)] == ' ') && tt.hint[:len(key)] == key {
				if key == tt.wantKey {
					found = true
				}
			}
		}
		if !found {
			t.Errorf("hint=%q: expected to match key=%q", tt.hint, tt.wantKey)
		}
	}
}

// TestFailHints_Lookup_NoFalseMatch 验证非精确匹配不会误匹配（W6-T2）。
// v0.3.0 fix (H4): 误匹配时记录为测试失败而非仅 log。
func TestFailHints_Lookup_NoFalseMatch(t *testing.T) {
	// "execution_error_something" 不应匹配 "execution_error"（非分隔符结尾）
	hint := "execution_error_something"
	falseMatch := false
	for key := range failHints {
		if len(hint) >= len(key) && hint[:len(key)] == key {
			// 只有后接 : 或空格才是合法匹配
			if len(hint) == len(key) || hint[len(key)] == ':' || hint[len(key)] == ' ' {
				falseMatch = true
				t.Errorf("hint=%q falsely matched key=%q (substring without delimiter)", hint, key)
			}
		}
	}
	if !falseMatch {
		t.Logf("hint=%q correctly did not match any key", hint)
	}
}

// TestSetGoalErrorHint_FillsSuggestions 验证 SetGoalErrorHint 自动查表补全（W6-T2）。
func TestSetGoalErrorHint_FillsSuggestions(t *testing.T) {
	h := NewHandler()
	h.Goals["goal-1"] = &GoalRecord{ID: "goal-1", Title: "test", Status: "failed"}

	// 传入空 suggestions → 自动从 failHints 查表
	h.SetGoalErrorHint("goal-1", "llm_timeout: deadline exceeded", nil)

	h.mu.RLock()
	g := h.Goals["goal-1"]
	h.mu.RUnlock()

	if g.ErrorHint != "llm_timeout: deadline exceeded" {
		t.Errorf("ErrorHint=%q, want %q", g.ErrorHint, "llm_timeout: deadline exceeded")
	}
	if len(g.Suggestions) == 0 {
		t.Fatal("SetGoalErrorHint MUST fill Suggestions from failHints")
	}
	foundSwitchModel := false
	for _, s := range g.Suggestions {
		if s.Action == "switch_model" {
			foundSwitchModel = true
		}
	}
	if !foundSwitchModel {
		t.Error("llm_timeout MUST suggest switch_model")
	}
}

// TestSetGoalErrorHint_DefaultSuggestions 验证无匹配时返回默认建议（W6-T2）。
func TestSetGoalErrorHint_DefaultSuggestions(t *testing.T) {
	h := NewHandler()
	h.Goals["goal-2"] = &GoalRecord{ID: "goal-2", Title: "test", Status: "failed"}

	// 传入未知 hint → 应该返回默认建议
	h.SetGoalErrorHint("goal-2", "unknown_error_type", nil)

	h.mu.RLock()
	g := h.Goals["goal-2"]
	h.mu.RUnlock()

	if len(g.Suggestions) == 0 {
		t.Fatal("SetGoalErrorHint MUST return default suggestions for unknown error")
	}
	if len(g.Suggestions) < 3 {
		t.Errorf("expected ≥3 default suggestions, got %d", len(g.Suggestions))
	}
}

// TestSetGoalErrorHint_UnknownGoal_SilentlyIgnored 验证不存在 goalID 时静默跳过。
// v0.3.0 fix (H4): 验证不存在的 goal 没有被意外创建。
func TestSetGoalErrorHint_UnknownGoal_SilentlyIgnored(t *testing.T) {
	h := NewHandler()
	// 不存在的 goalID——不应 panic，不应被创建
	h.SetGoalErrorHint("nonexistent", "error", nil)
	h.mu.RLock()
	_, exists := h.Goals["nonexistent"]
	h.mu.RUnlock()
	if exists {
		t.Error("SetGoalErrorHint MUST NOT create goal for nonexistent ID")
	}
}
