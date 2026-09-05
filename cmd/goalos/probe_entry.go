package main

import (
	"os"

	"github.com/goalos/goalos/internal/runtime"
)

// modebEntry __goalos-modeb 子命令入口（R-1664——模式 B 沙箱施加器）。
func modebEntry() { runtime.ModeBChildEntry(os.Args[2:]) }
