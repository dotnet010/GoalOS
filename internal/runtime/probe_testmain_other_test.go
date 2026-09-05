//go:build !linux

// probe_testmain_other_test.go——非 Linux 平台 TestMain（仅 __goalos-probe 拦截——
// 模式 B=Linux 专属；Windows=darwin 各自原生边界）。
package runtime

import (
	"os"
	"testing"

	"github.com/goalos/goalos/internal/runtime/probe"
)

// TestMain 拦截 __goalos-probe 子命令（先于测试运行）。
func TestMain(m *testing.M) {
	if len(os.Args) > 1 && os.Args[1] == "__goalos-probe" {
		os.Exit(probe.Main(os.Args[2:]))
	}
	os.Exit(m.Run())
}
