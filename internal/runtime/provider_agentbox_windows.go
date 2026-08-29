//go:build windows

// provider_agentbox_windows.go——Windows 探针形态（TC-RT-001a）。
package runtime

// probeWriteBin/Args fs 禁闭探针（写系统目录外——Restricted Token+ACL 面拒绝）。
func probeWriteBin() string    { return "cmd.exe" }
func probeWriteArgs() []string { return []string{"/c", "echo", "x", ">", "C:\\Windows\\goalos-probe-denied.txt"} }

// probeNetBin/Args 网络探针（NetworkBlocked 下出站必败）。
func probeNetBin() string { return "cmd.exe" }
func probeNetArgs() []string {
	return []string{"/c", "powershell", "-NoProfile", "-Command",
		"try { (New-Object Net.Sockets.TcpClient('192.0.2.1',80)); exit 1 } catch { exit 0 }"}
}
