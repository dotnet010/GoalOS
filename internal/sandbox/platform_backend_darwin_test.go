//go:build darwin

// platform_backend_darwin_test.go — 契约测试平台后端选择（darwin→Seatbelt，R-1399）。
package sandbox_test

import (
	"testing"

	"github.com/goalos/goalos/internal/sandbox"
)

// platformBackend 返回本平台的层级后端——darwin=DarwinBackend（Seatbelt 路径，
// R-1399 darwin-seatbelt 命名；macOS 诚实标注维持，R-1078）。
func platformBackend(t *testing.T) sandbox.Sandbox {
	t.Helper()
	return &sandbox.DarwinBackend{}
}
