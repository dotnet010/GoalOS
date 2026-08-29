//go:build windows

// bypass_probes_windows.go——windows 探针表（agentbox 承载——TC-RT-001a）。
// Restricted Token+Low IL+ACL 拒绝证据=「Access is denied」。
package runtime

import "path/filepath"

// platformProbeSet windows 探针组：fs 写区外/敏感目录读/出站连接。
func platformProbeSet(tc, home string) []struct{ name, binary, args string } {
	return []struct{ name, binary, args string }{
		{"direct write outside workspace", "cmd.exe", "/c echo x> " + filepath.Join(home, "goalos-bypass-probe-"+tc+".txt")},
		{"sensitive dir read ~/.ssh", "cmd.exe", "/c type " + filepath.Join(home, ".ssh", "config")},
		{"outbound connect 192.0.2.1:80", "powershell", "-NoProfile -Command try{(New-Object Net.Sockets.TcpClient('192.0.2.1',80))}catch{exit 1}"},
	}
}

// probeEchoBin Execute 正向探针（边界内真实执行证据——非空输出反虚假绿）。
func probeEchoBin() string { return "cmd.exe" }
