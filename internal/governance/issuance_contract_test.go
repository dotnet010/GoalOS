// issuance_contract_test.go——签发决策表契约测试（06 §1.3=R-1507 唯一权威；R-1601 签发不变量）。
// 标注=实现同步补强（非先红——诚实标注纪律：v0.3.1 W5 接线窗口落笔）。12 清单 G 节登记。
package governance

import (
	"path/filepath"
	"strings"
	"testing"
)

// TestGovernance_IssuanceDecisionTable 签发决策表逐行+序语义（行 4 风险级优先——RI-1 注记）。
func TestGovernance_IssuanceDecisionTable(t *testing.T) {
	cases := []struct {
		name    string
		in      IssuanceInput
		wantRRE bool
		wantMin string
	}{
		// 行 1：名单登记 ∧ caps⊆声明集 ∧ 无任意子进程 → {false, I1}
		{"行1-名单登记受限动作", IssuanceInput{WorkloadRegistered: true, CapsSubsetDeclared: true, RiskLevel: "R1"}, false, "I1"},
		// 行 1 排除：名单登记但含任意子进程 → 落行 2
		{"行1排除-名单内但shell", IssuanceInput{WorkloadRegistered: true, CapsSubsetDeclared: true, ArbitrarySubprocess: true, RiskLevel: "R1"}, true, "I2"},
		// 行 1 排除：声明集外扩 → 落行 2（未认证主体语义兜底）
		{"行2-名单外普通动作", IssuanceInput{WorkloadRegistered: false, CapsSubsetDeclared: true, RiskLevel: "R1"}, true, "I2"},
		{"行2-声明集外扩", IssuanceInput{WorkloadRegistered: true, CapsSubsetDeclared: false, RiskLevel: "R1"}, true, "I2"},
		// 行 3 拆分（R-1643——D-2 蓝图落地：废除涉网硬绑 I3，治理门替代档位门）：
		// 行 3s：敏感路径写入（目标路径感知）→ {true, I3, ""}
		{"行3s-敏感路径写入", IssuanceInput{WorkloadRegistered: false, CapsSubsetDeclared: true, SensitivePathWrite: true, RiskLevel: "R2"}, true, "I3"},
		// 行 3P：网络出站∩公网（或端点缺失保守）→ {true, I2, data_sharing}（治理门非档位门）
		{"行3P-网络出站公网", IssuanceInput{WorkloadRegistered: false, CapsSubsetDeclared: true, NetworkEgress: true, NetworkZone: "public", RiskLevel: "R2"}, true, "I2"},
		{"行3P-端点缺失保守", IssuanceInput{WorkloadRegistered: false, CapsSubsetDeclared: true, NetworkEgress: true, NetworkZone: "", RiskLevel: "R2"}, true, "I2"},
		// TC-RT-013 矩阵（R-1643-2 Kees 修正）：LAN 默认仍审查/信任后免除/回环恒免
		{"行3P-LAN默认审查", IssuanceInput{WorkloadRegistered: false, CapsSubsetDeclared: true, NetworkEgress: true, NetworkZone: "lan", TrustLAN: false, RiskLevel: "R2"}, true, "I2"},
		{"行3L-LAN信任免除", IssuanceInput{WorkloadRegistered: false, CapsSubsetDeclared: true, NetworkEgress: true, NetworkZone: "lan", TrustLAN: true, RiskLevel: "R2"}, true, "I2"},
		{"行3L-回环恒免", IssuanceInput{WorkloadRegistered: false, CapsSubsetDeclared: true, NetworkEgress: true, NetworkZone: "loopback", RiskLevel: "R2"}, true, "I2"},
		// 行 4：R≥4 或策略显式 → {true, I4}；RI-1：风险级优先于行 2/3（不被低档截获）
		{"行4-R4优先于行2", IssuanceInput{ArbitrarySubprocess: true, RiskLevel: "R4"}, true, "I4"},
		{"行4-R5优先于行3", IssuanceInput{NetworkEgress: true, RiskLevel: "R5"}, true, "I4"},
		{"行4-策略显式", IssuanceInput{PolicyExplicitI4: true, RiskLevel: "R1"}, true, "I4"},
		{"行4-L族残留兼容", IssuanceInput{ArbitrarySubprocess: true, RiskLevel: "L4"}, true, "I4"},
	}
	for _, c := range cases {
		got := ComputeIssuanceDecision(c.in)
		if got.RequiresRealEnforcement != c.wantRRE || got.MinIsolation != c.wantMin {
			t.Fatalf("%s：期望 {%v, %s}，实际 {%v, %s}", c.name, c.wantRRE, c.wantMin, got.RequiresRealEnforcement, got.MinIsolation)
		}
		// R-1643：行 3P（公网涉网）必须带 data_sharing 审批类型；其余行不得携带
		wantApproval := ""
		if strings.Contains(c.name, "行3P") {
			wantApproval = "data_sharing" // 行 3P=公网或 LAN 默认审查（TC-RT-013）
		}
		if got.ApprovalType != wantApproval {
			t.Fatalf("%s：ApprovalType 期望 %q，实际 %q", c.name, wantApproval, got.ApprovalType)
		}
		// R-1601 签发不变量：RRE=false ⇒ MinIsolation≤I1
		if !got.RequiresRealEnforcement && got.MinIsolation != "I1" && got.MinIsolation != "I0" {
			t.Fatalf("%s：R-1601 不变量违反——RRE=false 但 MinIsolation=%s", c.name, got.MinIsolation)
		}
	}
}

// TestGovernance_ClassifyActionAttrs 动作属性归类（两旗标事实源——R-1643 拆分后形态）。
func TestGovernance_ClassifyActionAttrs(t *testing.T) {
	asp, net := ClassifyActionAttrs("shell.execute", nil)
	if !asp || net {
		t.Fatal("shell.execute 应=任意子进程旗标，非网络")
	}
	asp2, net2 := ClassifyActionAttrs("browser.open", []string{"browser.open", "fs.write"})
	if asp2 || !net2 {
		t.Fatal("browser.open 应=网络出站旗标，非子进程")
	}
	asp3, net3 := ClassifyActionAttrs("fs.read", []string{"fs.read"})
	if asp3 || net3 {
		t.Fatal("fs.read 应=双旗标全否")
	}
}

// TestGovernance_IsSensitivePathWrite 敏感路径写入目标感知判定（R-1643 解冻——
// 目标路径落敏感目录族=敏感写入；工作区写入≠敏感写入）。
// 路径形态平台化（filepath.Join 构造——POSIX 字面量在 Windows 上形态失配事故实证：
// windows-daily CI 红出 2026-08-29）。
func TestGovernance_IsSensitivePathWrite(t *testing.T) {
	home := filepath.Join(string(filepath.Separator)+"home", "tester")
	if !IsSensitivePathWrite(filepath.Join(home, ".ssh", "id_rsa"), home) {
		t.Fatal(".ssh 下文件=敏感写入")
	}
	if !IsSensitivePathWrite(filepath.Join(home, ".goalos", "config", "x"), home) {
		t.Fatal(".goalos 下=敏感写入")
	}
	if IsSensitivePathWrite(filepath.Join(home, "Goals", "goal-1", "out.txt"), home) {
		t.Fatal("工作区写入≠敏感写入")
	}
	if IsSensitivePathWrite("", home) || IsSensitivePathWrite("/x", "") {
		t.Fatal("空输入=非敏感（防御性 false——无目标=无判定）")
	}
}
