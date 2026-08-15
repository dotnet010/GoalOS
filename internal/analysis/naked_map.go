// Package analysis — GoalOS 自定义 go/analysis 检查器（R-1449/R-1461 合并——W3-4 迁移任务）。
//
// 本包含 CI 检查的 go/analysis 实现——替代 shell 脚本的正则猜语法（发现 7/8/20 第三次实锤后
// 迁移定论）。检查器=语义级 AST 遍历，非文本模式匹配。
package analysis

import (
	"go/ast"
	"go/token"
	"go/types"

	"golang.org/x/tools/go/analysis"
	"golang.org/x/tools/go/analysis/passes/inspect"
	"golang.org/x/tools/go/ast/inspector"
)

// NakedMapAnalyzer — 裸可变 map 检测（R-1461 方案 3：真实赋值/mutation 追踪）。
//
// 契约：包级 var 声明的 map 类型——若该标识符在任何函数体内出现在赋值语句左侧
// （m[k]=v）或被传给 delete()，则判定为可变 map=FAIL。字面量初始化（含 {...}）不再
// 作为"只读"的代理特征——真实语义=是否被写。
var NakedMapAnalyzer = &analysis.Analyzer{
	Name:     "nakedmap",
	Doc:      "检查包级裸 map 的可变性——赋值/删除追踪（R-1461）",
	Requires: []*analysis.Analyzer{inspect.Analyzer},
	Run:      runNakedMap,
}

func runNakedMap(pass *analysis.Pass) (interface{}, error) {
	insp := pass.ResultOf[inspect.Analyzer].(*inspector.Inspector)

	// 第一步：收集包级 map 变量（TypesInfo 驱动——R-1463 发现 25 治本：穿透类型别名/
	// make() 调用/复合字面量/init() 延迟赋值——语法形态无关）。
	mapVars := make(map[string]token.Pos)
	scope := pass.Pkg.Scope()
	for _, name := range scope.Names() {
		v, ok := scope.Lookup(name).(*types.Var)
		if !ok {
			continue
		}
		// 穿透类型别名和具名类型直接拿到底层类型（Underlying()）
		if _, isMap := v.Type().Underlying().(*types.Map); isMap {
			mapVars[name] = v.Pos()
		}
	}

	if len(mapVars) == 0 {
		return nil, nil
	}

	// 第二步：追踪这些标识符的赋值/mutation——排除测试文件（测试中的局部 map 不检查）
	insp.Preorder([]ast.Node{(*ast.AssignStmt)(nil), (*ast.CallExpr)(nil)}, func(n ast.Node) {
		// 跳过测试文件
		pos := pass.Fset.Position(n.Pos())
		if len(pos.Filename) > 8 && pos.Filename[len(pos.Filename)-8:] == "_test.go" {
			return
		}
		switch stmt := n.(type) {
		case *ast.AssignStmt:
			// 检查左侧是否为包级 map 变量的索引赋值（m[k]=v）
			for _, lhs := range stmt.Lhs {
				if idx, ok := lhs.(*ast.IndexExpr); ok {
					if ident, ok := idx.X.(*ast.Ident); ok {
						if _, exists := mapVars[ident.Name]; exists {
							pass.Reportf(stmt.Pos(), "裸可变 map %s 被赋值（m[k]=v）——R-1461：包级 map 必须包私有化或使用 SafeMap 只读包装", ident.Name)
						}
					}
				}
			}
		case *ast.CallExpr:
			// 检查 delete(m, k) 调用
			if ident, ok := stmt.Fun.(*ast.Ident); ok && ident.Name == "delete" {
				if len(stmt.Args) > 0 {
					if arg, ok := stmt.Args[0].(*ast.Ident); ok {
						if _, exists := mapVars[arg.Name]; exists {
							pass.Reportf(stmt.Pos(), "裸可变 map %s 被 delete() 调用——R-1461：包级 map 必须包私有化或使用 SafeMap 只读包装", arg.Name)
						}
					}
				}
			}
		}
	})

	return nil, nil
}
