//go:build ignore

// main.go — go/analysis 检查器入口（R-1449/R-1461 合并——W3-4 迁移任务）。
//
// 用法：go run ./internal/analysis/ <pkg-path>——对指定包运行 nakedmap 分析器。
package main

import (
	"fmt"
	"os"

	"github.com/goalos/goalos/internal/analysis"
	"golang.org/x/tools/go/analysis/singlechecker"
)

func main() {
	singlechecker.Main(analysis.NakedMapAnalyzer)
	fmt.Fprintln(os.Stderr, "nakedmap analyzer exited")
}
