//go:build darwin

// bypass_probes_darwin.go——darwin 探针表（Option B 语义——会议 #256 定稿形态）。
package runtime

import "path/filepath"

// platformProbeSet darwin 探针组：写禁闭+敏感目录禁读+网络禁闭。
func platformProbeSet(tc, home string) []struct{ name, binary, args string } {
	return []struct{ name, binary, args string }{
		{"direct write outside workspace", "/usr/bin/touch", filepath.Join(home, ".goalos-bypass-probe-"+tc)},
		{"sensitive dir read ~/.ssh", "/bin/cat", filepath.Join(home, ".ssh")},
		{"outbound connect 127.0.0.1:9", "/usr/bin/nc", "-v -w 1 127.0.0.1 9"},
	}
}
