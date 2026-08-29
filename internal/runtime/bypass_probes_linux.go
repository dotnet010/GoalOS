//go:build linux

// bypass_probes_linux.go——linux 探针表（agentbox 承载——TC-RT-001b）。
// Landlock fs 禁闭+seccomp 网络过滤证据形态=EACCES「Permission denied」/EPERM/代理 denied。
package runtime

import "path/filepath"

// platformProbeSet linux 探针组：fs 写区外/敏感目录读/出站连接（deny 证据=EACCES 族）。
func platformProbeSet(tc, home string) []struct{ name, binary, args string } {
	return []struct{ name, binary, args string }{
		{"direct write outside workspace", "/usr/bin/touch", filepath.Join(home, ".goalos-bypass-probe-"+tc)},
		{"sensitive dir read ~/.ssh", "/bin/cat", filepath.Join(home, ".ssh", "id_rsa")},
		{"outbound connect 192.0.2.1:80", "/usr/bin/nc", "-w 1 192.0.2.1 80"},
	}
}

// probeEchoBin Execute 正向探针（边界内真实执行证据——非空输出反虚假绿）。
func probeEchoBin() string { return "/bin/echo" }
