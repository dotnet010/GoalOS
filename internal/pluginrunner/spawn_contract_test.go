// 契约测试：统一 spawn 原语+DLL 劫持对抗（R-1086/R-1089——会议 #194）。
//
// 断言来源: R-1086（统一 spawn 原语四防线——创建期 O_CLOEXEC CI AST/启动期
//   close_range+4.19 /proc 兜底/CPython bpo-47260 无条件 fallback 教训/spawn 期唯一入口/
//   Windows HANDLE_LIST 白名单）；R-1089（DLL 四件套：SetDefaultDllDirectories+只读执行
//   目录+writable∩dll-search=∅ 编译期契约+加载路径审计）。
//
// 当前契约形态: spawn 原语骨架阶段（transport_unix.go 已落 fd 继承收口）。本测试
// 以 AST 扫描断言已落防线：
//   - spawn 期唯一入口（R-1086 ③）：模块内 os/exec.Command 直调仅允许出现在
//     executor.go（散落直调=CI FAIL）；
//   - Windows 传输层（named pipe 路径）不产生额外 spawn 点（transport_windows.go
//     零 exec.Command 直调——HANDLE_LIST 白名单路径无旁路）。
//
// 纪律: AST 行为断言（go/parser——编译期结构，非源码文本 grep）。

package pluginrunner_test

import (
	"go/ast"
	"go/parser"
	"go/token"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// scanExecCommandSites 解析 pluginrunner 模块全部非测试 Go 源文件，返回
// 出现 os/exec Command 调用（exec.Command( / cmd.Command(）的文件名集合。
// 测试文件（*_test.go）合法 spawn 辅助子进程（对抗夹具）——不参与唯一入口统计。
func scanExecCommandSites(t *testing.T) map[string]int {
	t.Helper()
	root := "."
	all, err := filepath.Glob(filepath.Join(root, "*.go"))
	if err != nil || len(all) == 0 {
		t.Fatalf("前置: 模块源文件枚举失败（%v, %d 个文件）", err, len(all))
	}
	files := []string{}
	for _, f := range all {
		if strings.HasSuffix(f, "_test.go") {
			continue // 测试夹具 spawn 合法——唯一入口契约仅约束生产代码
		}
		files = append(files, f)
	}
	sites := map[string]int{}
	for _, f := range files {
		fset := token.NewFileSet()
		tree, err := parser.ParseFile(fset, f, nil, 0)
		if err != nil {
			t.Fatalf("前置: 解析 %s 失败: %v", f, err)
		}
		ast.Inspect(tree, func(n ast.Node) bool {
			call, ok := n.(*ast.CallExpr)
			if !ok {
				return true
			}
			sel, ok := call.Fun.(*ast.SelectorExpr)
			if !ok {
				return true
			}
			ident, ok := sel.X.(*ast.Ident)
			if ok && (ident.Name == "exec" || ident.Name == "cmd") && sel.Sel.Name == "Command" {
				sites[filepath.Base(f)]++
			}
			return true
		})
	}
	return sites
}

// TestSpawn_SingleEntryPoint — spawn 期全模块唯一入口（R-1086 ③）。
// 断言：exec.Command 直调仅允许出现在 executor.go（spawn 唯一入口）——
// 散落直调=fd 卫生防线旁路。
func TestSpawn_SingleEntryPoint(t *testing.T) {
	sites := scanExecCommandSites(t)
	if len(sites) == 0 {
		t.Error("MUST 1（R-1086 ③）: 模块内无 exec.Command 调用——spawn 唯一入口缺失（执行路径断裂）")
		return
	}
	for file, count := range sites {
		if file != "executor.go" {
			t.Errorf("MUST 1（R-1086 ③）: %s 存在 %d 处 exec.Command 直调——spawn 期唯一入口违约（散落直调=fd 卫生防线旁路）", file, count)
		}
	}
	if _, ok := sites["executor.go"]; !ok {
		t.Error("MUST 1（R-1086 ③）: executor.go 无 exec.Command 调用——唯一入口锚点缺失")
	}
	// MUST 2（R-1086 ③）: 唯一入口存在时，其余文件直调数为 0（上循环已断言——
	// 此处复核总数=executor.go 计数，防止集合语义误判）。
	total := 0
	for _, c := range sites {
		total += c
	}
	if sites["executor.go"] != total {
		t.Errorf("MUST 2（R-1086 ③）: spawn 点计数异常——executor.go=%d 总数=%d（旁路直调未归口）", sites["executor.go"], total)
	}
}

// TestSandbox_Adversarial_Windows_DLLHijack — Windows spawn 旁路闭合（R-1089 前置）。
// 断言：Windows 传输层（named pipe 路径——transport_windows.go）不产生额外
// spawn 点（零 exec.Command 直调）——HANDLE_LIST 白名单路径无进程创建旁路
// （dll-search 攻击面不因传输层扩大）。跨平台可执行：AST 解析不依赖构建标签。
func TestSandbox_Adversarial_Windows_DLLHijack(t *testing.T) {
	// Windows 红出定位（2026-08-17）: transport_windows.go 位于 ipc/ 子包——
	// 此前 glob 本包 *.go 永远找不到该文件，Windows 平台下误报"载体缺失"。
	files, err := filepath.Glob(filepath.Join("ipc", "*.go"))
	if err != nil {
		t.Fatalf("前置: 模块源文件枚举失败: %v", err)
	}
	for _, f := range files {
		if filepath.Base(f) != "transport_windows.go" {
			continue
		}
		fset := token.NewFileSet()
		tree, err := parser.ParseFile(fset, f, nil, 0)
		if err != nil {
			t.Fatalf("前置: 解析 %s 失败: %v", f, err)
		}
		count := 0
		ast.Inspect(tree, func(n ast.Node) bool {
			call, ok := n.(*ast.CallExpr)
			if !ok {
				return true
			}
			sel, ok := call.Fun.(*ast.SelectorExpr)
			if !ok {
				return true
			}
			ident, ok := sel.X.(*ast.Ident)
			if ok && (ident.Name == "exec" || ident.Name == "cmd") && sel.Sel.Name == "Command" {
				count++
			}
			return true
		})
		if count != 0 {
			t.Errorf("MUST 1（R-1089）: transport_windows.go 存在 %d 处 exec.Command 直调——named pipe 传输路径产生进程创建旁路（dll-search 攻击面扩大）", count)
		}
		// MUST 2（R-1089）: Windows 传输层必须存在（四件套的编译载体——文件缺席=契约对象缺失）。
		t.Logf("transport_windows.go 解析完成（exec.Command 直调=%d）", count)
		return
	}
	// 文件缺席：Windows 构建下无传输载体——契约对象缺失。
	if runtime.GOOS == "windows" {
		t.Error("MUST 2（R-1089）: Windows 平台下 transport_windows.go 缺席——named pipe 传输载体缺失")
	}
}
