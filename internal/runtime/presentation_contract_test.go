// presentation_contract_test.go——TC-RT-100（12 清单 F 节——CLI 诚实呈现）：
// 四平台快照+grep 断言=人类可读通道零工程词汇（R-1512-1 范围=人类可读通道；
// R-1554 内嵌常量纪律+R-1477 词汇纪律+R-1479 macOS 措辞+R-1550 行 2 警告样式）。
// 标注=实现同步补强（非先红——诚实标注纪律）。
package runtime

import (
	"strings"
	"testing"
)

// 工程词汇黑名单（TC-RT-100 grep 断言集——人类可读通道禁止出现）。
var engineeringVocabulary = []string{
	"T0", "T1", "T2", "T3", "I0", "I1", "I2", "I3", "I4", "I5",
	"Seatbelt", "seccomp", "Landlock", "sandbox-exec", "gVisor",
	"Namespace", "cgroup", "BPF", "Seatbelt",
}

// TestRuntime_CLI_HonestTierPresentation（TC-RT-100）：
// (1)四平台快照（darwin/linux/windows/xinchuang）——产品词呈现；
// (2)人类可读通道零工程词汇（黑名单 grep）；
// (3)macOS 措辞（R-1479——纵深防御+弃用组件诚实标注）；
// (4)行 2 警告样式标记（R-1550）；(5)拒绝句（行 5/6 合一——不虚构依据）；
// (6)未知档位 fail-closed 最低呈现。
func TestRuntime_CLI_HonestTierPresentation(t *testing.T) {
	platforms := []string{"darwin", "linux", "windows", "xinchuang"}
	for _, platform := range platforms {
		sel := Selection{Tier: TierRestricted, Reason: "row3_restricted"}
		p := PresentRuntime(sel, platform, nil)
		// (1)产品词呈现
		if !strings.Contains(p.LevelLine, "受限") {
			t.Fatalf("平台 %s：LevelLine 应含产品词「受限」，实际 %q", platform, p.LevelLine)
		}
		// (2)零工程词汇（grep 断言——人类可读面全字段）
		humanFace := p.LevelLine + p.Reason + strings.Join(p.Degraded, ",") + p.TierLabel
		for _, word := range engineeringVocabulary {
			if strings.Contains(humanFace, word) {
				t.Fatalf("平台 %s：人类可读面含工程词汇 %q（TC-RT-100 词汇纪律违反）——%q", platform, word, humanFace)
			}
		}
		// (3)macOS 措辞
		if platform == "darwin" && !strings.Contains(p.LevelLine, "纵深防御") {
			t.Fatalf("darwin：LevelLine 应含「纵深防御」诚实标注，实际 %q", p.LevelLine)
		}
		if platform != "darwin" && !strings.Contains(p.LevelLine, "原生强制隔离") {
			t.Fatalf("平台 %s：LevelLine 应含「原生强制隔离」，实际 %q", platform, p.LevelLine)
		}
	}

	// (4)行 2 警告样式（名单缺失/过期=配置异常不以普通行伪装）
	row2 := PresentRuntime(Selection{Tier: TierRestricted, Reason: "row2_restricted"}, "linux", nil)
	if !row2.ReasonWarning {
		t.Fatal("行 2 命中必须标记警告样式（R-1550）")
	}
	if !strings.Contains(row2.Reason, "名单登记缺失或已过期") {
		t.Fatalf("行 2 依据句=映射表行 2 文本，实际 %q", row2.Reason)
	}
	row3 := PresentRuntime(Selection{Tier: TierRestricted, Reason: "row3_restricted"}, "linux", nil)
	if row3.ReasonWarning {
		t.Fatal("行 3 命中不得标记警告样式")
	}

	// (5)拒绝句（i4_backend_unavailable→行 5/6 合一人类句）
	rej := PresentRuntime(Selection{Tier: TierRestricted, Reason: "i4_backend_unavailable"}, "darwin", nil)
	if !strings.Contains(rej.Reason, "已拒绝") {
		t.Fatalf("拒绝族应呈现「已拒绝」句，实际 %q", rej.Reason)
	}

	// (6)未知档位 fail-closed=最低呈现（不虚构高档位）
	unknown := PresentRuntime(Selection{Tier: ExecutionTier(99), Reason: "unknown_key"}, "linux", nil)
	if unknown.TierLabel != "受限" {
		t.Fatalf("未知档位应 fail-closed 呈现「受限」，实际 %q", unknown.TierLabel)
	}

	// (7)JSON 机器值枚举封闭（04 §14.3）
	for tier, want := range map[ExecutionTier]string{
		TierT0: "cooperative", TierRestricted: "constrained", TierHardenedLocal: "hardened_local",
	} {
		if got := TierJSONValue(tier); got != want {
			t.Fatalf("TierJSONValue(%v)=%q，期望 %q", tier, got, want)
		}
	}
}
