//go:build linux || windows

// goruntime_helper_test.go——测试辅助（GOOS 读取——linux||windows 文件族共享）。
package runtime

import goruntime "runtime"

func goruntimeGOOS() string { return goruntime.GOOS }
