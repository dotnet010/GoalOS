//go:build linux

// bypass_probes_linux.go——linux 探针表（R-1666 原生探针形态——ERRNO 数字证据，
// 本地化文本零依赖）。
package runtime

import (
	"os"
	"path/filepath"
)

// platformProbeSet linux 探针组（原生探针：写区外/敏感目录读/出站连接——
// 拒绝证据=PROBE-ERRNO 非零）。
func platformProbeSet(tc, home string) []struct{ name, binary, args string } {
	self, err := os.Executable()
	if err != nil {
		self = "/bin/false"
	}
	return []struct{ name, binary, args string }{
		{"direct write outside workspace", self, "__goalos-probe write " + filepath.Join(home, ".goalos-bypass-probe-"+tc)},
		{"sensitive dir read ~/.ssh", self, "__goalos-probe read " + filepath.Join(home, ".ssh", "id_rsa")},
		{"outbound connect 192.0.2.1:80", self, "__goalos-probe dial 192.0.2.1:80"},
	}
}

// probeEchoBin Execute 正向探针（边界内真实执行证据——非空输出反虚假绿）。
func probeEchoBin() string { return "/bin/echo" }
