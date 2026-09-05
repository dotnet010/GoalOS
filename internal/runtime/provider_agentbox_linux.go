//go:build linux

// provider_agentbox_linux.go——Linux 探针形态（TC-RT-001b）。
// R-1666 原生探针化：self re-exec + __goalos-probe 子命令——ERRNO 数字证据，
// 本地化文本/shell 依赖全退役（C locale 假设同刀消灭）。
package runtime

import (
	"os"
	"path/filepath"
)

// probeWriteBin/Args fs 禁闭探针（原生探针写 home——真实 Landlock 覆盖面；
// DAC 放行而边界必拒=探测力真实。S-266-01+R-1666）。
func probeWriteBin() string {
	self, err := os.Executable()
	if err != nil {
		return "/bin/false" // 不可用=探针必败=fail-closed 方向
	}
	return self
}
func probeWriteArgs() []string {
	home, err := os.UserHomeDir()
	if err != nil || home == "" {
		home = "/tmp"
	}
	return []string{"__goalos-probe", "write", filepath.Join(home, ".goalos-precheck-probe")}
}

// probeNetBin/Args 网络探针（原生 dial 探针——NetworkBlocked 下出站必败）。
func probeNetBin() string { return probeWriteBin() }
func probeNetArgs() []string {
	return []string{"__goalos-probe", "dial", "192.0.2.1:80"}
}
