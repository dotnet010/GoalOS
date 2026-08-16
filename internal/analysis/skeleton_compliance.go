// skeleton_compliance.go — Skeleton[T] 合规检查（R-1473——会议 #226 发现 35）。
//
// 契约：检测骨架函数是否返回规范 ErrNotImplemented 而非临时拼出的 fmt.Errorf 文本——
// CI 在写下第一个不合规文件时就拦住（约定+靠记忆遵守路线第四次失效规模更大=CI 机检化
// 必须落地非文档约定）。检测形态：函数体含"骨架"或"未实现"字样的 fmt.Errorf 调用=
// 不合规（应返回包级 ErrNotImplemented）。
package analysis

import (
	"go/ast"
	"strings"

	"golang.org/x/tools/go/analysis"
	"golang.org/x/tools/go/analysis/passes/inspect"
	"golang.org/x/tools/go/ast/inspector"
)

// SkeletonComplianceAnalyzer — Skeleton[T] 合规检查（R-1473）。
//
// 契约：函数体含"骨架"或"未实现"字样的 fmt.Errorf 调用=不合规（应返回包级
// ErrNotImplemented——R-1468 统一纪律）。
var SkeletonComplianceAnalyzer = &analysis.Analyzer{
	Name:     "skeletoncompliance",
	Doc:      "检查骨架函数是否返回规范 ErrNotImplemented 而非临时拼出的 fmt.Errorf 文本（R-1473）",
	Requires: []*analysis.Analyzer{inspect.Analyzer},
	Run:      runSkeletonCompliance,
}

func runSkeletonCompliance(pass *analysis.Pass) (interface{}, error) {
	insp := pass.ResultOf[inspect.Analyzer].(*inspector.Inspector)

	insp.Preorder([]ast.Node{(*ast.CallExpr)(nil)}, func(n ast.Node) {
		call := n.(*ast.CallExpr)
		// 检查是否为 fmt.Errorf 调用
		sel, ok := call.Fun.(*ast.SelectorExpr)
		if !ok {
			return
		}
		ident, ok := sel.X.(*ast.Ident)
		if !ok || ident.Name != "fmt" || sel.Sel.Name != "Errorf" {
			return
		}
		// 检查第一个参数（格式字符串）是否含"骨架"或"未实现"字样
		if len(call.Args) == 0 {
			return
		}
		if lit, ok := call.Args[0].(*ast.BasicLit); ok {
			if strings.Contains(lit.Value, "骨架") || strings.Contains(lit.Value, "未实现") {
				pass.Reportf(call.Pos(), "骨架函数返回临时拼出的 fmt.Errorf 文本——应返回包级 ErrNotImplemented（R-1468 统一纪律，R-1473 CI 机检化）")
			}
		}
	})

	return nil, nil
}
