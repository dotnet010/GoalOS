//go:build ignore

// main.go — go/analysis 检查器入口（R-1449/R-1461 合并——W3-4 迁移任务）。
//
// 用法：go run ./internal/analysis/ <pkg-path>——对指定包运行 nakedmap 分析器。
package main

import (
	"fmt"
	"os"

	"github.com/goalos/goalos/internal/analysis"
	"golang.org/x/tools/go/analysis/multichecker"
)

func main() {
	// 两个分析器：nakedmap（R-1461 方案 3）+ skeletoncompliance（R-1473）
	// multichecker 同时运行两个分析器
	multichecker.Main(analysis.NakedMapAnalyzer, analysis.SkeletonComplianceAnalyzer)
	fmt.Fprintln(os.Stderr, "nakedmap analyzer exited")
}
