// detect_platform.go——平台实际达成 IsolationLevel 探测（06 §1.3 平台矩阵映射——
// R-1506 阶梯+探测纪律：空跑验证不信静态读数 R-960；能力探测失败=降级证据）。
// 任务 5.5 前置接线（R-1640②）；TC-RT-050 平台适配矩阵实机验证的数据源。
package runtime

import (
	"os/exec"
	goruntime "runtime"

	"github.com/goalos/goalos/internal/sandbox"
)

// DetectPlatformIsolation 本机达成的 I 级（06 §1.3 平台×层级可达矩阵——诚实标注）：
// darwin=sandbox-exec 在→I2（Seatbelt 约定级——R-1571/R-1641），不在→I1
// linux=Landlock+seccomp-notify 齐→I3；否则→I2（seccomp BPF 自加载基线——信创同档）
// windows=JobObject 可用→I2；否则→I1
// 探测失败/未知平台=I1（fail-closed 保守下限）。
func DetectPlatformIsolation() IsolationLevel {
	switch goruntime.GOOS {
	case "darwin":
		if _, err := exec.LookPath("sandbox-exec"); err == nil {
			return I2
		}
		return I1
	case "linux":
		caps, err := sandbox.Detect()
		if err != nil {
			return I1
		}
		if caps.LandlockABI >= 1 && caps.SeccompNotify {
			return I3
		}
		return I2
	case "windows":
		caps, err := sandbox.Detect()
		if err != nil {
			return I1
		}
		if caps.JobObject {
			return I2
		}
		return I1
	default:
		return I1
	}
}
