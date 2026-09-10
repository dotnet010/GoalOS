//go:build darwin

// platform_matrix_darwin_test.go——TC-RT-050 darwin 面（任务 5.6——平台适配矩阵实机验证：
// 矩阵各平台实际生效级别与承诺一致——R-1481-6）。darwin 本机实证链：
// 探测=I2 ∧ Provider 达成=I2 ∧ 呈现=「受限」 ∧ 06 §1.3 macOS 行承诺=I2 四面一致。
package runtime

import (
	"context"
	"strings"
	"testing"
)

// TestRuntime_PlatformMatrix_Darwin（TC-RT-050 darwin 面——实机验证）：
// (1)探测层：DetectPlatformIsolation=I2（sandbox-exec 在位实证）；
// (2)Provider 层：darwinSeatbelt 达成=I2（与探测一致——无降级证据）；
// (3)呈现层：受限档呈现=「受限（纵深防御——含已弃用系统组件的诚实标注）」（R-1479 措辞）；
// (4)矩阵承诺一致性：06 §1.3 macOS 行=I2——探测/达成/呈现/承诺四面同源一致。
// 反虚假绿：任一面漂移=FAIL（探测说 I2 而 Provider 只给 I1=呈现不一致事故类）。
func TestRuntime_PlatformMatrix_Darwin(t *testing.T) {
	ctx := context.Background()

	// (1)探测层
	detected := DetectPlatformIsolation()
	if detected != I2 {
		t.Fatalf("(1)探测层应=I2（sandbox-exec 在位），实际 %s", detected)
	}

	// (2)Provider 层（真实边界——Precheck 三探针已过=OS 强制实证非声称）
	p := NewDarwinSeatbeltProvider(t.TempDir(), t.TempDir())
	if err := p.Prepare(ctx, RuntimePlan{PlanID: "matrix-test", Tier: TierRestricted}); err != nil {
		t.Fatalf("(2)Provider Prepare 失败: %v", err)
	}
	caps, err := p.Capabilities(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if caps.AchievedIsolation != I2 {
		t.Fatalf("(2)Provider 达成应=I2，实际 %s（与探测漂移=事故类）", caps.AchievedIsolation)
	}
	if len(caps.DegradedEvidence) != 0 {
		t.Fatalf("(2)darwin 受限档应零降级证据（Seatbelt 要么在要么不在），实际: %v", caps.DegradedEvidence)
	}

	// (3)呈现层（产品语义+R-1479 措辞）
	pres := PresentRuntime(Selection{Tier: TierRestricted, Reason: "row3_restricted"}, "darwin", caps.DegradedEvidence)
	if !strings.Contains(pres.LevelLine, "受限") || !strings.Contains(pres.LevelLine, "纵深防御") {
		t.Fatalf("(3)呈现层应=受限+纵深防御注记（R-1479），实际 %q", pres.LevelLine)
	}

	// (4)四面一致（探测 I2=达成 I2=呈现受限=矩阵承诺 I2——任一面漂移即事故）
	if detected != caps.AchievedIsolation {
		t.Fatalf("(4)矩阵一致性违反：探测 %s ≠ Provider 达成 %s", detected, caps.AchievedIsolation)
	}
}
