//go:build windows

// provider_agentbox_windows.go——Windows 探针形态（TC-RT-001a）。
// R-1666 原生探针化：self re-exec + __goalos-probe——ERRNO 数字证据
//（PowerShell/cmd 探针退役——企业组策略 ExecutionPolicy/AppLocker/CLM 面）。
package runtime

import (
	"os"
	"path/filepath"
)

// probeWriteBin/Args fs 禁闭探针（原生探针写 home——受限档 DenyWrite=home 契约面）。
func probeWriteBin() string {
	self, err := os.Executable()
	if err != nil {
		return `C:\Windows\System32\cmd.exe` // 不可用=探针必败方向（exec 失败=非零退出）
	}
	return self
}
func probeWriteArgs() []string {
	home, err := os.UserHomeDir()
	if err != nil || home == "" {
		home = os.Getenv("USERPROFILE")
	}
	return []string{"__goalos-probe", "write", filepath.Join(home, "goalos-precheck-probe.txt")}
}

// probeNetBin/Args 网络探针（原生 dial——NetworkBlocked 下出站必败）。
func probeNetBin() string { return probeWriteBin() }
func probeNetArgs() []string {
	return []string{"__goalos-probe", "dial", "192.0.2.1:80"}
}
