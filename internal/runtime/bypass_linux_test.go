//go:build linux

// bypass_linux_test.go——TC-RT-001b（12 清单 F 节）：Linux 受限档旁路双态断言。
//
// 2026-08-31 实机裁决（Ubuntu 24.04 真机 goalos-test+GH hosted runner run #72
// 双重实证）：agentbox 边界依赖 userns，Ubuntu 24.04 默认 AppArmor 策略
// （kernel.apparmor_restrict_unprivileged_userns=1）阻断→agentbox fail-open
// 裸跑。处置=双态断言（非跳过——测试在 CI 照跑）：
//   - 平台有能力 → 全探针矩阵必须全拒（原断言强度不降）；
//   - 平台无能力 → Precheck 必须 fail-closed（「边界失效」签名——S-266-02 落地），
//     裸跑被执行=红（不可能到绿）。
package runtime

import "testing"

func TestRuntime_Bypass_SyscallDenied_Linux(t *testing.T) {
	runBypassProbeDual(t, "TC-RT-001b", NewAgentboxProvider(t.TempDir(), t.TempDir(), "linux"))
}
