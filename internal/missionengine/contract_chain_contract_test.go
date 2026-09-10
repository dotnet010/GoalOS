// contract_chain_contract_test.go——TestCompletionContract_VersionChain（12 清单 G 节——
// 任务 5.8；规格=05 §6 R-1597 合并规则唯一权威）。标注=实现同步补强（非先红——诚实标注）。
// 断言：继承锚点只增不减（缺任一=fail-closed）/版本单调/supersedes 链/旧版本终态/首版前置。
package missionengine

import (
	"strings"
	"testing"
)

// TestCompletionContract_VersionChain 版本链合并规则断言（R-1597——继承锚点只增不减）。
func TestCompletionContract_VersionChain(t *testing.T) {
	chain := NewContractChain()
	criteria := CompletionCriteria{GoalType: "research", SuccessDefinition: "产出调研报告"}

	// (5)无首版修订=错误（修订不能凭空）
	if _, err := chain.Revise("goal-x", criteria, []string{"预算≤10k"}); err == nil {
		t.Fatal("(5)无首版修订必须失败（Record 先行）")
	}

	// 首版登记
	v1 := chain.Record("goal-1", criteria, []string{"预算≤10k", "不改生产库"})
	if v1.Version != 1 || v1.Status != ContractActive || v1.Supersedes != "" {
		t.Fatalf("首版形态违反：version=%d status=%s supersedes=%q", v1.Version, v1.Status, v1.Supersedes)
	}

	// (1)锚点缺失=fail-closed（已 Frozen 审批约束只增不减）
	_, err := chain.Revise("goal-1", criteria, []string{"预算≤10k"}) // 缺「不改生产库」
	if err == nil || !strings.Contains(err.Error(), "不改生产库") {
		t.Fatalf("(1)锚点缺失必须 fail-closed 且点名缺失锚点，实际: %v", err)
	}

	// (2)(3)修订成功路径：锚点全携+新增锚点（只增不减）+版本递增+supersedes 链+旧版本终态
	v2, err := chain.Revise("goal-1", criteria, []string{"预算≤10k", "不改生产库", "交付前人工复核"})
	if err != nil {
		t.Fatalf("(2)锚点全携修订应成功: %v", err)
	}
	if v2.Version != 2 || v2.Status != ContractRevised || v2.Supersedes != v1.ContractID {
		t.Fatalf("(3)修订版本链形态违反：version=%d status=%s supersedes=%q（应=2/revised/%s）",
			v2.Version, v2.Status, v2.Supersedes, v1.ContractID)
	}
	if v1.Status != ContractSuperseded {
		t.Fatalf("(3)旧版本应转终态 Superseded，实际 %s", v1.Status)
	}
	if len(v2.FrozenAnchors) != 3 {
		t.Fatalf("(1)锚点集应=3（只增不减含新增），实际 %d", len(v2.FrozenAnchors))
	}

	// (4)链连续性：Latest=v2，链长=2
	if chain.Latest("goal-1") != v2 {
		t.Fatal("(4)Latest 必须=链尾修订版")
	}
	if chain.ChainLength("goal-1") != 2 {
		t.Fatalf("(4)链长=2，实际 %d", chain.ChainLength("goal-1"))
	}
}
