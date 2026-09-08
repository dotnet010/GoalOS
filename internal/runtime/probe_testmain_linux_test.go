//go:build linux

// probe_testmain_linux_test.go——测试二进制子命令拦截（re-exec 自身=探针/沙箱
// 载体零外部二进制——R-1666）。daemon 侧同标记拦截=cmd/goalos/main.go。
package runtime

import (
	"os"
	"testing"

	"github.com/goalos/goalos/internal/runtime/probe"
	"github.com/zhangyunhao116/agentbox"
)

// TestMain 拦截 __goalos-probe/__goalos-modeb 子命令（先于测试运行）。
// 最前=agentbox.MaybeSandboxInit()（R-1678——agentbox 沙箱子进程=re-exec 自身
// 二进制+环境标记，子进程必须最前调用它才进入沙箱初始化——缺失=子进程落入
// m.Run() 跑全量测试=递归爆炸+fail-open 裸跑，双机实机实锤）。
func TestMain(m *testing.M) {
	if agentbox.MaybeSandboxInit() {
		return
	}
	if len(os.Args) > 1 {
		switch os.Args[1] {
		case "__goalos-probe":
			os.Exit(probe.Main(os.Args[2:]))
		case modeBMarker:
			modeBChildMain(os.Args[2:]) // 不返回
		}
	}
	os.Exit(m.Run())
}
