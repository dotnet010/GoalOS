//go:build linux

// provider_agentbox_linux.go——Linux 探针形态（TC-RT-001b）。
package runtime

import (
	"os"
	"path/filepath"
)

// probeWriteBin/Args fs 禁闭探针（写用户 home——真实 Landlock 覆盖面；
// 2026-08-31 修正：原写 /usr=DAC 平凡拒（非 root 必拒=不证 Landlock 在位），
// Ubuntu 24.04 实机暴露（userns 被 AppArmor 默认策略阻断→agentbox fail-open
// 裸跑——Precheck 假绿、主探针抓获真实旁路）。home 写=受限档契约必拒面
// （DenyWrite=home 语义）——DAC 放行而边界必拒，探测力真实。
// S-266-01 兑现。
func probeWriteBin() string { return "/usr/bin/touch" }
func probeWriteArgs() []string {
	home, err := os.UserHomeDir()
	if err != nil || home == "" {
		home = "/tmp" // 兜底=/tmp 也不在工作区（受限档 tmpDir 为独立临时目录）——仍非平凡拒
	}
	return []string{filepath.Join(home, ".goalos-precheck-probe")}
}

// probeNetBin/Args 网络探针（NetworkBlocked 下出站必败）。
func probeNetBin() string { return "/usr/bin/nc" }
func probeNetArgs() []string {
	return []string{"-w", "1", "192.0.2.1", "80"}
}
