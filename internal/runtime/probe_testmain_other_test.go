//go:build !linux

// probe_testmain_other_test.go——非 Linux 平台 TestMain（__goalos-probe 探针+
// __goalos-fd3d 转发器双拦截——FD3 测试二进制内自举面；模式 B=Linux 专属）。
package runtime

import (
	"os"
	"testing"

	"github.com/goalos/goalos/internal/fd3"
	"github.com/goalos/goalos/internal/runtime/probe"
)

// TestMain 拦截 __goalos-probe/__goalos-fd3d 子命令（先于测试运行）。
func TestMain(m *testing.M) {
	if len(os.Args) > 1 {
		switch os.Args[1] {
		case "__goalos-probe":
			os.Exit(probe.Main(os.Args[2:]))
		case "__goalos-fd3d":
			os.Exit(fd3.ForwarderMain(os.Args[2:]))
		}
	}
	os.Exit(m.Run())
}
