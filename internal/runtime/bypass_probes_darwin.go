//go:build darwin

// bypass_probes_darwin.go——darwin 探针表（R-1666 原生探针形态——ERRNO 数字证据，
// 本地化文本零依赖）。2026-09-10 实机漂移抓获：共享断言 R-1666 化（PROBE-ERRNO
// 数字证据族——文本证据全退役）后 darwin 滞留系统二进制探针（touch/cat/nc——
// 无 PROBE-ERRNO 行=形态漂移红）。迁移前提=受限 profile 放行 Go 运行时引导面
// （sysctl-read hw.pagesize——profile_darwin_restricted.sb 同刀，实机实证）。
package runtime

import (
	"os"
	"path/filepath"
)

// platformProbeSet darwin 探针组（原生探针：写区外/敏感目录读/出站连接——
// 拒绝证据=PROBE-ERRNO 非零；出站目标=192.0.2.1:80 TEST-NET-1 保留段零路由风险）。
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
