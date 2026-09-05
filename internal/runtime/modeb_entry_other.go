//go:build !linux

package runtime

import (
	"fmt"
	"os"
)

// ModeBChildEntry 非 Linux=模式 B 不存在（fail-closed——不应被调到）。
func ModeBChildEntry(_ []string) {
	fmt.Fprintln(os.Stderr, "MODEB-FATAL: 模式 B 仅 Linux 可用（免 userns 基座——R-1664）")
	os.Exit(2)
}
