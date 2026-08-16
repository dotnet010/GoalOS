//go:build windows || xinchuang

// platform_backend_other_test.go — 契约测试平台后端选择
// （windows/xinchuang→MXC stub——跨平台 fail-closed 契约载体，R-958）。
package sandbox_test

import (
	"testing"

	"github.com/goalos/goalos/internal/sandbox"
)

// platformBackend 返回本平台的层级后端——windows/xinchuang=MXCBackend
// （ProcessContainer 调用不实现——R-958 stub；fail-closed 契约不变）。
func platformBackend(t *testing.T) sandbox.Sandbox {
	t.Helper()
	return &sandbox.MXCBackend{}
}
