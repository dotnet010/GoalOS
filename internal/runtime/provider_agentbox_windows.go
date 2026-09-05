//go:build windows

// provider_agentbox_windows.go——Windows 探针形态（TC-RT-001a）。
package runtime

import (
	"os"
	"path/filepath"
)

// probeWriteBin/Args fs 禁闭探针（写用户 home——真实边界覆盖面；
// 2026-08-31 修正：原写 C:\Windows=管理员面 DAC 拒（不证受限令牌边界在位）。
// home 写=受限档契约必拒面（DenyWrite=home——F1 语义）。S-266-01 同源修正。
func probeWriteBin() string { return "cmd.exe" }
func probeWriteArgs() []string {
	home, err := os.UserHomeDir()
	if err != nil || home == "" {
		home = os.Getenv("USERPROFILE")
	}
	probe := filepath.Join(home, "goalos-precheck-probe.txt")
	return []string{"/c", "echo", "x", ">", probe}
}

// probeNetBin/Args 网络探针（NetworkBlocked 下出站必败）。
// 退出码约定=Precheck 契约（0=泄漏/非0=拒绝——与 fs 探针同族；2026-08-31 实机
// 修正：原脚本 try{exit 1}catch{exit 0}=极性反置——边界成立时 catch 出口 0
// 被 Precheck 误判「失效」，env 门禁遮蔽下潜伏。顺带实锤：exit code 语义必须
// 实测，不写脚本想当然）。
func probeNetBin() string { return "cmd.exe" }
func probeNetArgs() []string {
	return []string{"/c", "powershell", "-NoProfile", "-Command",
		"try { (New-Object Net.Sockets.TcpClient('192.0.2.1',80)); Write-Output 'NET-LEAK'; exit 0 } catch { Write-Output ('NET-DENIED: ' + $_.Exception.Message); exit 1 }"}
}
