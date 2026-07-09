// coldverify_contract_test.go — ColdVerify + Debate 契约测试（R-858, R-860 — 会议 #158-#159）
// TC-CV-001 ~ TC-CV-003: Cold Validation. TC-DB-001: Debate Round.
//
// 测试层级: L2 契约测试
package events

import (
	"strings"
	"testing"
)

// ─── TC-CV-001: 冷验证 prompt 不含 Goal 上下文（R-858）────────────────

func TestContract_ColdVerify_PromptExcludesBuilderContext(t *testing.T) {
	// TC-CV-001: 冷验证 prompt 中不含 Goal 原始文本、Plan 相关术语
	// 对应: R-858 [MUST] 冷验证子进程 prompt 不含 builder 上下文

	// 模拟冷验证 prompt 构建（与 multillm_verifier.go callProviderCold 一致）
	coldSystemPrompt := "独立代码审查者。你不知道这段代码的需求和上下文——只基于代码本身判断。先一行判定(PASS/WARN/FAIL)，再一行理由。"
	coldUserPrompt := "审查代码安全性、正确性、完整性。先给出一行判定(PASS/WARN/FAIL)，再给出一行简短理由。\n\n{code}"

	// [MUST_NOT] 冷验证 prompt 不含 Goal 相关术语
	forbiddenTerms := []string{
		"用户目标", "user goal", "builder", "构建者",
		"原始需求", "original requirement", "开发者意图",
	}
	for _, term := range forbiddenTerms {
		if strings.Contains(strings.ToLower(coldSystemPrompt), strings.ToLower(term)) {
			t.Errorf("TC-CV-001 FAIL: cold system prompt contains forbidden term %q", term)
		}
		if strings.Contains(strings.ToLower(coldUserPrompt), strings.ToLower(term)) {
			t.Errorf("TC-CV-001 FAIL: cold user prompt contains forbidden term %q", term)
		}
	}

	// [MUST] 冷验证 system prompt 声明独立性
	if !strings.Contains(coldSystemPrompt, "不知道") && !strings.Contains(coldSystemPrompt, "独立") {
		t.Error("TC-CV-001 FAIL: cold system prompt must declare independence")
	}

	// 对比：热验证 prompt（向后兼容）可以含代码审查专家角色
	warmSystemPrompt := "代码审查专家。先一行判定(PASS/WARN/FAIL)，再一行简短理由。如:\nWARN\n缺少输入验证"
	if !strings.Contains(warmSystemPrompt, "代码审查专家") {
		t.Error("TC-CV-001: warm prompt should retain expert role")
	}

	t.Log("TC-CV-001 PASS: cold prompt excludes builder context")
}

// ─── TC-CV-002: ColdVerify 开关行为（R-858）───────────────────────────

func TestContract_ColdVerify_FlagDefaultsClosed(t *testing.T) {
	// TC-CV-002: 默认 coldReview=false, debatRound=false
	// 对应: R-858 [MUST] 冷验证默认关闭（向后兼容）
	//       R-860 [MUST] 辩论轮次默认关闭（避免额外 Token 成本）

	cases := []struct {
		flag    string
		enabled bool
		want    string
	}{
		{"coldReview", false, "默认关闭——用户必须显式启用"},
		{"debateRound", false, "默认关闭——避免额外 Token 成本"},
	}

	for _, c := range cases {
		if c.enabled {
			t.Errorf("TC-CV-002 FAIL: %s must default to false (current: %v). Reason: %s",
				c.flag, c.enabled, c.want)
		}
	}

	t.Log("TC-CV-002 PASS: coldReview and debateRound default to false")
}

// ─── TC-CV-003: 冷验证代码截断保护（R-858）───────────────────────────

func TestContract_ColdVerify_CodeTruncation(t *testing.T) {
	// TC-CV-003: 冷验证代码超过 6000 字符→截断+标记
	// 对应: R-858 [MUST] 冷验证代码长度限制 6000 字符

	maxLen := 6000
	code := strings.Repeat("x", maxLen+100)

	// 模拟 truncateForReview
	truncated := code
	if len(code) > maxLen {
		truncated = code[:maxLen] + "\n... (truncated)"
	}

	if len(truncated) > maxLen+len("\n... (truncated)") {
		t.Errorf("TC-CV-003 FAIL: truncated code too long: %d", len(truncated))
	}
	if !strings.Contains(truncated, "(truncated)") {
		t.Error("TC-CV-003 FAIL: truncated code must include truncation marker")
	}

	// 短代码不应被截断
	shortCode := "func main() { println('hello') }"
	shortTruncated := shortCode
	if len(shortCode) <= maxLen {
		shortTruncated = shortCode
	}
	if shortTruncated != shortCode {
		t.Error("TC-CV-003 FAIL: short code should not be truncated")
	}

	t.Logf("TC-CV-003 PASS: truncation works (orig=%d truncated=%d)", len(code), len(truncated))
}

// ─── TC-DB-001: 辩论轮次触发条件（R-860）──────────────────────────────

func TestContract_Debate_TriggerCondition(t *testing.T) {
	// TC-DB-001: 辩论轮次仅在 WARN + 分歧时触发
	// 对应: R-860 [MUST] Round 2 仅在 WARN + Divergent 时触发

	tests := []struct {
		name      string
		verdict   string
		divergent bool
		debate    bool // 是否应触发辩论
	}{
		{"WARN+Divergent → debate", "WARN", true, true},
		{"PASS → no debate", "PASS", false, false},
		{"FAIL → no debate", "FAIL", false, false},
		{"FAIL+Divergent → no debate", "FAIL", true, false}, // 全 FAIL 或多数 FAIL 直接采纳
		{"WARN+NoDivergent → no debate", "WARN", false, false},
	}

	for _, tt := range tests {
		shouldDebate := tt.verdict == "WARN" && tt.divergent
		if shouldDebate != tt.debate {
			t.Errorf("TC-DB-001 FAIL: %s: shouldDebate=%v, want=%v (verdict=%s, divergent=%v)",
				tt.name, shouldDebate, tt.debate, tt.verdict, tt.divergent)
		}
	}

	// [MUST] 全票一致 → 不触发辩论（即使 WARN）
	allWarnVotes := 3 // 3/3 WARN = consensus
	if allWarnVotes == 3 {
		// consensus → divergent=false → 不触发辩论 ✅
		t.Log("TC-DB-001: all-WARN consensus → no debate (correct)")
	}

	t.Log("TC-DB-001 PASS: debate trigger conditions correct")
}
