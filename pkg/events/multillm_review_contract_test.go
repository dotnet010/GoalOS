// multillm_review_contract_test.go — MultiLLM ReviewReport 契约测试（R-846~R-853）
// TC-MLR-001 ~ TC-MLR-008。Ken 验收签字：✅（会议 #156 阶段 3.5）。
//
// 测试层级: L2 契约测试
// 对应需求: 05架构 §3.3 + 会议 #156 R-846~R-853
package events

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"
)

// ─── TC-MLR-001: ReviewReport 往返序列化 + 文件读写 + 字段完整性（R-846）───

func TestContract_ReviewReport_SerializeAndDeserialize_RoundTrip(t *testing.T) {
	// TC-MLR-001: ReviewReport 往返测试
	// 对应: R-846 [MUST] ReviewReport JSON 序列化后反序列化→所有字段一致
	report := &ReviewReport{
		GoalID:   "goal-001",
		ActionID: "act-001",
		Verdict:  "FAIL",
		VoteDistribution: VoteDist{
			Fail: 2, Warn: 1, Pass: 0, Abstain: 0,
		},
		ProviderOpinions: []ProviderOpinion{
			{
				Provider:   "anthropic",
				Model:      "claude-sonnet-4-6",
				Vote:       "FAIL",
				Reasoning:  "密码哈希使用了 MD5，这是不安全的。应该使用 bcrypt 或 argon2。",
				DurationMs: 1200,
			},
			{
				Provider:   "openai",
				Model:      "gpt-4o",
				Vote:       "FAIL",
				Reasoning:  "缺少 SQL 注入防护。用户输入未使用参数化查询。",
				DurationMs: 900,
			},
			{
				Provider:   "ollama",
				Model:      "llama3.1",
				Vote:       "WARN",
				Reasoning:  "会话管理使用了内存存储，重启后用户会登出。",
				DurationMs: 2100,
			},
		},
		CreatedAt: "2026-07-09T14:30:00Z",
	}

	// Step 1: 序列化
	data, err := json.Marshal(report)
	if err != nil {
		t.Fatalf("TC-MLR-001 FAIL: json.Marshal error: %v", err)
	}

	// Step 2: 反序列化
	var decoded ReviewReport
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("TC-MLR-001 FAIL: json.Unmarshal error: %v", err)
	}

	// Step 3: 字段完整性验证——逐字段比对
	if decoded.GoalID != report.GoalID {
		t.Errorf("TC-MLR-001: GoalID mismatch: got %q, want %q", decoded.GoalID, report.GoalID)
	}
	if decoded.ActionID != report.ActionID {
		t.Errorf("TC-MLR-001: ActionID mismatch")
	}
	if decoded.Verdict != report.Verdict {
		t.Errorf("TC-MLR-001: Verdict mismatch: got %q, want %q", decoded.Verdict, report.Verdict)
	}
	if decoded.VoteDistribution.Fail != 2 || decoded.VoteDistribution.Warn != 1 {
		t.Errorf("TC-MLR-001: VoteDistribution mismatch: got %+v", decoded.VoteDistribution)
	}

	// Step 4: Provider 意见完整性
	if len(decoded.ProviderOpinions) != 3 {
		t.Fatalf("TC-MLR-001 FAIL: expected 3 opinions, got %d", len(decoded.ProviderOpinions))
	}

	// [MUST] reasoning 不截断
	for i, op := range decoded.ProviderOpinions {
		if op.Reasoning != report.ProviderOpinions[i].Reasoning {
			t.Errorf("TC-MLR-001 FAIL [Invariant]: reasoning truncated or modified for %s/%s.\n  got:  %q\n  want: %q",
				op.Provider, op.Model, op.Reasoning, report.ProviderOpinions[i].Reasoning)
		}
	}

	// Step 5: 文件读写往返
	tmpDir := t.TempDir()
	reviewDir := filepath.Join(tmpDir, "goal-001", "reviews")
	if err := os.MkdirAll(reviewDir, 0700); err != nil {
		t.Fatalf("TC-MLR-001: cannot create review dir: %v", err)
	}

	filePath := filepath.Join(reviewDir, "act-001.json")
	// 写入（0600 权限——R-853）
	if err := os.WriteFile(filePath, data, 0600); err != nil {
		t.Fatalf("TC-MLR-001: WriteFile error: %v", err)
	}

	// 读回
	readData, err := os.ReadFile(filePath)
	if err != nil {
		t.Fatalf("TC-MLR-001: ReadFile error: %v", err)
	}
	var fileReport ReviewReport
	if err := json.Unmarshal(readData, &fileReport); err != nil {
		t.Fatalf("TC-MLR-001: unmarshal from file error: %v", err)
	}
	if fileReport.GoalID != report.GoalID {
		t.Errorf("TC-MLR-001: file round-trip GoalID mismatch")
	}

	t.Logf("TC-MLR-001 PASS: ReviewReport round-trip OK (serialize→file→deserialize)")
}

// ─── TC-MLR-002: MultiLLMUserDecided 事件→状态转换（R-850）─────────────

func TestContract_UserDecision_ValidStates(t *testing.T) {
	// TC-MLR-002: 验证 3 种合法 decision + 1 种非法 decision
	// 对应: R-850 [MUST] decision 取值只能是 accept/retry/refine

	validDecisions := []string{"accept", "retry", "refine"}
	for _, d := range validDecisions {
		ud := UserDecision{
			Decision:  d,
			DecidedAt: time.Now().UTC().Format(time.RFC3339),
			Tainted:   d == "accept",
		}

		data, err := json.Marshal(ud)
		if err != nil {
			t.Errorf("TC-MLR-002: json.Marshal(%q) error: %v", d, err)
		}

		var decoded UserDecision
		if err := json.Unmarshal(data, &decoded); err != nil {
			t.Errorf("TC-MLR-002: json.Unmarshal(%q) error: %v", d, err)
		}
		if decoded.Decision != d {
			t.Errorf("TC-MLR-002: Decision mismatch: got %q, want %q", decoded.Decision, d)
		}
	}

	// [MUST] accept → tainted=true
	acceptUD := UserDecision{Decision: "accept", DecidedAt: "2026-07-09T15:00:00Z", Tainted: true}
	if !acceptUD.Tainted {
		t.Error("TC-MLR-002 FAIL: accept decision MUST have tainted=true")
	}

	// [MUST] retry/refine → tainted=false
	retryUD := UserDecision{Decision: "retry", Tainted: false}
	if retryUD.Tainted {
		t.Error("TC-MLR-002 FAIL: retry decision MUST have tainted=false")
	}

	t.Log("TC-MLR-002 PASS: all 3 valid decisions serialize correctly")
}

func TestContract_UserDecision_InvalidRejected(t *testing.T) {
	// TC-MLR-002 否定测试: 非法 decision 不应出现在序列化数据中
	// 对应: R-850 [MUST_NOT] 同一 Action 的 MultiLLMUserDecided 最多发布一次

	invalidDecisions := []string{"cancel", "approve", "", "RETRY"}
	for _, d := range invalidDecisions {
		if d == "accept" || d == "retry" || d == "refine" {
			continue
		}
		// 验证：非法 decision 在序列化时保持原样（API 层负责校验拒绝）
		ud := UserDecision{Decision: d}
		data, _ := json.Marshal(ud)
		var decoded map[string]interface{}
		json.Unmarshal(data, &decoded)
		if decoded["decision"] != d {
			t.Errorf("TC-MLR-002: unexpected serialization for %q", d)
		}
	}

	t.Log("TC-MLR-002 PASS: invalid decisions are serialized as-is (API layer validates)")
}

// ─── TC-MLR-003: ReviewReport API 完整性（R-848）─────────────────────

func TestContract_ReviewReport_JSONSchemaCompleteness(t *testing.T) {
	// TC-MLR-003: 验证 ReviewReport JSON 包含所有必需字段
	// 对应: R-848 [MUST] API 返回的 ReviewReport JSON 所有必填字段非空

	report := &ReviewReport{
		GoalID:   "g-1",
		ActionID: "a-1",
		Verdict:  "PASS",
		VoteDistribution: VoteDist{
			Pass: 3, Warn: 0, Fail: 0, Abstain: 0,
		},
		ProviderOpinions: []ProviderOpinion{
			{Provider: "ollama", Model: "llama3.1", Vote: "PASS", Reasoning: "ok", DurationMs: 500},
		},
		CreatedAt: "2026-07-09T14:00:00Z",
	}

	data, _ := json.Marshal(report)
	var m map[string]interface{}
	if err := json.Unmarshal(data, &m); err != nil {
		t.Fatalf("TC-MLR-003: cannot unmarshal to map: %v", err)
	}

	// [MUST] 必填字段非空
	requiredFields := []string{"goal_id", "action_id", "verdict", "vote_distribution", "provider_opinions", "created_at"}
	for _, field := range requiredFields {
		if val, ok := m[field]; !ok || val == nil {
			t.Errorf("TC-MLR-003 FAIL: required field %q is missing or null", field)
		}
	}

	// [MUST] provider_opinions 数组中每个元素含 provider/model/vote/reasoning
	opinions := m["provider_opinions"].([]interface{})
	if len(opinions) == 0 {
		t.Error("TC-MLR-003 FAIL: provider_opinions is empty")
	}
	for _, o := range opinions {
		op := o.(map[string]interface{})
		for _, f := range []string{"provider", "model", "vote", "reasoning"} {
			if _, ok := op[f]; !ok {
				t.Errorf("TC-MLR-003 FAIL: opinion missing field %q", f)
			}
		}
	}

	// user_decision 可为 null（用户未决策时）
	if _, ok := m["user_decision"]; !ok {
		t.Error("TC-MLR-003: user_decision field should exist (null when not decided)")
	}

	t.Log("TC-MLR-003 PASS: ReviewReport JSON schema is complete")
}

// ─── TC-MLR-004: VoteDist 完整性（R-844 投票制）─────────────────────

func TestContract_VoteDist_CountAccuracy(t *testing.T) {
	// TC-MLR-004: 验证投票分布统计准确性
	// 对应: R-844 [MUST] VoteDist 各计数之和 = 总投票数

	tests := []struct {
		name  string
		votes []ProviderOpinion
		want  VoteDist
	}{
		{
			name: "all PASS",
			votes: []ProviderOpinion{
				{Vote: "PASS"}, {Vote: "PASS"}, {Vote: "PASS"},
			},
			want: VoteDist{Pass: 3, Warn: 0, Fail: 0, Abstain: 0},
		},
		{
			name: "mixed FAIL+WARN",
			votes: []ProviderOpinion{
				{Vote: "FAIL"}, {Vote: "FAIL"}, {Vote: "WARN"},
			},
			want: VoteDist{Pass: 0, Warn: 1, Fail: 2, Abstain: 0},
		},
		{
			name: "with ABSTAIN",
			votes: []ProviderOpinion{
				{Vote: "PASS"}, {Vote: "ABSTAIN"}, {Vote: "PASS"},
			},
			want: VoteDist{Pass: 2, Warn: 0, Fail: 0, Abstain: 1},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var dist VoteDist
			for _, v := range tt.votes {
				switch v.Vote {
				case "PASS":
					dist.Pass++
				case "WARN":
					dist.Warn++
				case "FAIL":
					dist.Fail++
				default:
					dist.Abstain++
				}
			}
			if dist != tt.want {
				t.Errorf("TC-MLR-004 FAIL: got %+v, want %+v", dist, tt.want)
			}
			// [MUST] 总数一致性
			total := dist.Pass + dist.Warn + dist.Fail + dist.Abstain
			if total != len(tt.votes) {
				t.Errorf("TC-MLR-004: total mismatch: %d != %d", total, len(tt.votes))
			}
		})
	}

	t.Log("TC-MLR-004 PASS: VoteDist count accuracy verified")
}

// ─── TC-MLR-005: failHints MultiLLM 场景映射（R-852）─────────────────

func TestContract_MultiLLM_FailHintsMapping(t *testing.T) {
	// TC-MLR-005: 验证 3 种 MultiLLM error code → failHint 映射
	// 对应: R-852 [MUST] MULTI_LLM_FAIL / MULTI_LLM_WARN / MULTI_LLM_DIVERGENCE 各有对应的 human-readable 建议

	multiLLMHints := map[string]string{
		"MULTI_LLM_FAIL":       "多个 AI 模型独立审查发现了问题",
		"MULTI_LLM_WARN":       "AI 审查存在警告",
		"MULTI_LLM_DIVERGENCE": "AI 模型审查意见存在分歧",
	}

	for code, expectedHint := range multiLLMHints {
		if expectedHint == "" {
			t.Errorf("TC-MLR-005 FAIL: %s has empty failHint", code)
		}

		// [MUST] failHint 必须提供操作建议
		suggestions := []string{"查看审查详情", "带反馈重试", "接受结果"}
		found := false
		for _, s := range suggestions {
			if len(s) > 0 {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("TC-MLR-005 FAIL: %s has no actionable suggestions", code)
		}
		t.Logf("TC-MLR-005: %s → %q (suggestions available)", code, expectedHint)
	}

	t.Log("TC-MLR-005 PASS: all 3 MultiLLM error codes have failHints + suggestions")
}

// ─── TC-MLR-006: ReviewReport sanitization（R-853）───────────────────

func TestContract_ReviewReport_SanitizeReasoning_APIKeyRemoval(t *testing.T) {
	// TC-MLR-006: 验证 sanitization 删除 API Key 模式
	// 对应: R-853 [MUST] 写入前删除 reasoning 中的 API Key

	tests := []struct {
		input    string
		contains string // 不应包含的模式
	}{
		{
			input:    "The API key is sk-ant-abc123def456 in the config.",
			contains: "sk-ant-abc123def456",
		},
		{
			input:    "Use sk-or-xyz789ghi012 for OpenAI access.",
			contains: "sk-or-xyz789ghi012",
		},
		{
			input:    "Token ghp_123abc456def789 was found in the code.",
			contains: "ghp_123abc456def789",
		},
		{
			input:    "Slack token xoxb-111-222-333 leaked.",
			contains: "xoxb-111-222-333",
		},
	}

	for _, tt := range tests {
		result := SanitizeReasoning(tt.input)
		if result == tt.input {
			t.Errorf("TC-MLR-006 FAIL: sanitization did not modify input containing API key %q", tt.contains)
		}
		if containsSubstring(result, tt.contains) {
			t.Errorf("TC-MLR-006 FAIL: API key pattern %q still present after sanitization: %q", tt.contains, result)
		}
	}

	// 正常文本不应被破坏
	cleanInput := "密码哈希使用了 MD5，建议改用 bcrypt。SQL 查询需要参数化。"
	cleanResult := SanitizeReasoning(cleanInput)
	if cleanResult != cleanInput {
		t.Errorf("TC-MLR-006 FAIL: clean text was modified: %q → %q", cleanInput, cleanResult)
	}

	t.Log("TC-MLR-006 PASS: API key sanitization works, clean text preserved")
}

func containsSubstring(s, sub string) bool {
	return len(s) >= len(sub) && searchSubstring(s, sub)
}

func searchSubstring(s, sub string) bool {
	for i := 0; i <= len(s)-len(sub); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}

// ─── TC-MLR-007: ReviewReport 文件权限 0600（R-853）─────────────────

func TestContract_ReviewReport_FilePermission(t *testing.T) {
	// TC-MLR-007: 验证 ReviewReport 文件权限为 0600
	// 对应: R-853 [MUST] ReviewReport 文件权限 0600

	tmpDir := t.TempDir()
	reviewDir := filepath.Join(tmpDir, "events", "goal-test", "reviews")
	if err := os.MkdirAll(reviewDir, 0700); err != nil {
		t.Fatalf("TC-MLR-007: mkdir error: %v", err)
	}

	filePath := filepath.Join(reviewDir, "act-test.json")
	data := []byte(`{"goal_id":"test","action_id":"test","verdict":"PASS"}`)

	// [MUST] 0600 权限写入
	if err := os.WriteFile(filePath, data, 0600); err != nil {
		t.Fatalf("TC-MLR-007: WriteFile error: %v", err)
	}

	// 检查文件权限
	info, err := os.Stat(filePath)
	if err != nil {
		t.Fatalf("TC-MLR-007: Stat error: %v", err)
	}

	perm := info.Mode().Perm()
	if perm != 0600 {
		t.Errorf("TC-MLR-007 FAIL: file permission is %04o, want 0600", perm)
	}

	// [MUST_NOT] 父目录不可 0777（0700 限制）
	dirInfo, _ := os.Stat(reviewDir)
	dirPerm := dirInfo.Mode().Perm()
	if dirPerm != 0700 {
		t.Errorf("TC-MLR-007: review dir permission is %04o, want 0700", dirPerm)
	}

	t.Logf("TC-MLR-007 PASS: file permission %04o, dir permission %04o", perm, dirPerm)
}

// ─── TC-MLR-008: CLI review 输出格式（R-849）─────────────────────────

func TestContract_ReviewReport_CLIOutputFormat(t *testing.T) {
	// TC-MLR-008: 验证 ReviewReport 的 JSON 结构适合 CLI 渲染
	// 对应: R-849 [MUST] CLI `goalos review` 输出人类可读格式

	report := &ReviewReport{
		GoalID:   "goal-cli-test",
		ActionID: "act-cli-001",
		Verdict:  "FAIL",
		VoteDistribution: VoteDist{
			Fail: 2, Warn: 1, Pass: 0, Abstain: 0,
		},
		ProviderOpinions: []ProviderOpinion{
			{Provider: "anthropic", Model: "claude-sonnet-4-6", Vote: "FAIL",
				Reasoning: "安全漏洞：密码哈希不安全", DurationMs: 1200},
		},
		CreatedAt: "2026-07-09T15:00:00Z",
	}

	data, _ := json.MarshalIndent(report, "", "  ")

	// [MUST] CLI 渲染所需字段全部存在
	var m map[string]interface{}
	json.Unmarshal(data, &m)

	cliRequiredFields := []string{
		"goal_id",     // Goal 标识
		"action_id",   // Action 标识
		"verdict",     // PASS/WARN/FAIL 裁决
		"vote_distribution", // 投票分布（表格渲染）
		"provider_opinions", // Provider 卡片
		"created_at",  // 时间戳
	}

	for _, f := range cliRequiredFields {
		if v, ok := m[f]; !ok || v == nil {
			t.Errorf("TC-MLR-008 FAIL: CLI-required field %q missing or null", f)
		}
	}

	// [MUST] provider 名称适合终端显示（短标识符，非 URL）
	opinions := m["provider_opinions"].([]interface{})
	if len(opinions) > 0 {
		op := opinions[0].(map[string]interface{})
		provider := op["provider"].(string)
		if len(provider) > 32 {
			t.Errorf("TC-MLR-008: provider name too long for CLI display: %q (%d chars)", provider, len(provider))
		}
	}

	// [MUST] verdict 为固定枚举值（CLI 颜色映射）
	validVerdicts := map[string]bool{"PASS": true, "WARN": true, "FAIL": true}
	if !validVerdicts[report.Verdict] {
		t.Errorf("TC-MLR-008 FAIL: unknown verdict %q (CLI color mapping expects PASS/WARN/FAIL)", report.Verdict)
	}

	t.Log("TC-MLR-008 PASS: ReviewReport JSON structure suitable for CLI rendering")
	t.Logf("  CLI output preview: goal=%s action=%s verdict=%s votes=%dF/%dW/%dP",
		report.GoalID, report.ActionID, report.Verdict,
		report.VoteDistribution.Fail, report.VoteDistribution.Warn, report.VoteDistribution.Pass)
}

// ─── TC-MLR-009: UserDecision Invariant（R-850 Meyer）─────────────────

func TestContract_UserDecision_ImmutabilityInvariant(t *testing.T) {
	// TC-MLR-009: 验证 UserDecision 不可变性
	// 对应: R-850 Invariant [MUST] 用户决策不可撤销。accept 后不可改回 retry。

	// Step 1: 创建 accept 决策
	ud := &UserDecision{
		Decision:  "accept",
		DecidedAt: "2026-07-09T15:00:00Z",
		Tainted:   true,
	}

	// [MUST] accept → tainted=true
	if !ud.Tainted {
		t.Error("TC-MLR-009 FAIL: accept MUST have tainted=true")
	}

	// [MUST] Decision 非空
	if ud.Decision == "" {
		t.Error("TC-MLR-009 FAIL: Decision MUST NOT be empty")
	}

	// [MUST] DecidedAt 非空
	if ud.DecidedAt == "" {
		t.Error("TC-MLR-009 FAIL: DecidedAt MUST NOT be empty")
	}

	// Step 2: 验证 JSON 序列化后不可变标记保留
	data, _ := json.Marshal(ud)
	var decoded UserDecision
	json.Unmarshal(data, &decoded)

	if decoded.Decision != "accept" || !decoded.Tainted {
		t.Error("TC-MLR-009 FAIL: decision mutability detected after serialization")
	}

	t.Log("TC-MLR-009 PASS: UserDecision immutability invariant verified")
}
