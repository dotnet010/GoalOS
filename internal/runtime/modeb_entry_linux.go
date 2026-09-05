//go:build linux

package runtime

// ModeBChildEntry daemon __goalos-modeb 子命令入口（导出窄口——cmd/goalos 用）。
func ModeBChildEntry(args []string) { modeBChildMain(args) }
