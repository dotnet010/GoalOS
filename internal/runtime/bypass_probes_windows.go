//go:build windows

// bypass_probes_windows.go——windows 探针表（R-1666 原生探针形态——ERRNO 数字
// 证据；PowerShell/cmd 探针退役——企业组策略 ExecutionPolicy/AppLocker/CLM 面）。
package runtime

import (
	"os"
	"path/filepath"
)

// platformProbeSet windows 探针组（原生探针：写区外/敏感目录读/出站连接）。
func platformProbeSet(tc, home string) []struct{ name, binary, args string } {
	self, err := os.Executable()
	if err != nil {
		self = `C:\Windows\System32\cmd.exe` // 不可用=探针必败方向
	}
	return []struct{ name, binary, args string }{
		{"direct write outside workspace", self, "__goalos-probe write " + filepath.Join(home, "goalos-bypass-probe-"+tc+".txt")},
		{"sensitive dir read ~/.ssh", self, "__goalos-probe read " + filepath.Join(home, ".ssh", "config")},
		{"outbound connect 192.0.2.1:80", self, "__goalos-probe dial 192.0.2.1:80"},
	}
}
