//go:build linux

// bypass_linux_test.go——TC-RT-001b（12 清单 F 节——TestRuntime_Bypass_SyscallDenied_Linux）：
// 直接系统调用旁路拒绝（Linux seccomp+ns 族）：同上断言（seccomp clone 旗标级 BPF 过滤
// +namespace+cgroup——信创同族）。先红=W1；转绿=W5-6（任务 5.2 收敛）。
// 注（Linux 专属 CI 纪律）：本测试在 darwin 不编译——Linux CI 绿≠darwin 本地覆盖，
// 转绿时必须 Linux runner 实证（指令级模拟+sudo 重试协议兜底）。
package runtime

import "testing"

func TestRuntime_Bypass_SyscallDenied_Linux(t *testing.T) {
	runBypassProbe(t, "TC-RT-001b")
}
