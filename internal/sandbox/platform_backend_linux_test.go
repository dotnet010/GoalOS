//go:build linux && !xinchuang

// platform_backend_linux_test.go — 契约测试平台后端选择（linux→gVisor，R-1078）。
package sandbox_test

import (
	"testing"

	"github.com/goalos/goalos/internal/sandbox"
)

// platformBackend 返回本平台的层级后端——linux= gVisor（I4 主路径，R-1078）。
func platformBackend(t *testing.T) sandbox.Sandbox {
	t.Helper()
	return sandbox.NewGVisorBackend()
}
