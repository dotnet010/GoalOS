// 契约测试：目标偏离检测（R-1082——会议 #193 威胁模型升级）。
//
// 断言来源: R-1082（确定性层零误报+ESN+CUSUM 语义层遥测；信号只升级不自动阻断；
//   drift→statefs rollback→重跑闭环）。
//
// 当前契约形态: 两层检测实现归 5.13/5.14（W5-6）——Detector 处于骨架期。
//   骨架期契约=R-1468/R-1473 拍板形态：Check 返回 Skeleton[*DriftSignal]——
//   Unwrap 必须失败（未实现裁决不可信）、Direction 必须 FailClosed（漂移检测
//   缺席时不得放行）、TaskRef 承载转绿引用。5.13 转绿实现时本测试断言升级为
//   确定性层零误报+语义层遥测的完整行为。
//
// 纪律: 行为断言——禁止源码文本断言；逐条 t.Error 非 FailNow。

package drift_test

import (
	"testing"

	"github.com/goalos/goalos/internal/drift"
	"github.com/goalos/goalos/internal/skeleton"
)

// TestDriftDetector_DeterministicLayer_ZeroFalsePositive — 骨架期 fail-closed 契约
// （R-1082/R-1468）。断言：未实现的检测器不得产出"无误报"的裁决值——Unwrap
// 失败=裁决不可信；FailClosed 方向=漂移检测缺席时信号消费方不得视作"无偏离"。
func TestDriftDetector_DeterministicLayer_ZeroFalsePositive(t *testing.T) {
	det := &drift.Detector{}
	sk, err := det.Check(
		drift.GoalDeclarationSet{GoalID: "g-1", DeclaredCapabilities: []string{"fs.read"}, DeclaredTargets: []string{"t-1"}},
		drift.ToolCallSet{ActionID: "a-1", InvokedCapabilities: []string{"fs.read"}, InvokedTargets: []string{"t-1"}},
	)
	if err != nil {
		t.Errorf("MUST（R-1468）: 骨架期 Check 不得返回非骨架形态错误——Skeleton 承载语义: %v", err)
	}
	if _, uerr := sk.Unwrap(); uerr == nil {
		t.Error("MUST（R-1082/R-1468）: 骨架期 Unwrap 必须失败——未实现的裁决值不可信（零误报不成立）")
	}
	if sk.Direction() != skeleton.FailClosed {
		t.Errorf("MUST（R-1082/R-1468）: 骨架期方向=%s，必须为 FailClosed——漂移检测缺席不得放行", sk.Direction())
	}
	if sk.TaskRef() == "" {
		t.Error("MUST（R-1468）: TaskRef 必须非空——转绿任务引用承载于类型（非注释）")
	}
}
