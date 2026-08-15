// 契约测试：统一 spawn 原语+DLL 劫持对抗（R-1086/R-1089——会议 #194）。
//
// 断言来源: R-1086（统一 spawn 原语四防线——创建期 O_CLOEXEC CI AST/启动期
//   close_range+4.19 /proc 兜底/CPython bpo-47260 无条件 fallback 教训/spawn 期唯一入口/
//   Windows HANDLE_LIST 白名单）；R-1089（DLL 四件套：SetDefaultDllDirectories+只读执行
//   目录+writable∩dll-search=∅ 编译期契约+加载路径审计）。
//
// 先红状态: spawn 原语骨架阶段（transport_unix.go 已落 fd 继承收口）。
// 转绿任务: 3.19（统一 spawn 原语实现）。

package pluginrunner_test

import "testing"

// TestSpawn_SingleEntryPoint — spawn 期全模块唯一入口（R-1086 ③）。
// 断言：散落 os/exec 直调=CI FAIL（check-spawn-entrypoint.sh 或 AST 扫描）。
func TestSpawn_SingleEntryPoint(t *testing.T) {
	t.Skip("先红挂起（R-571）——转绿归任务 3.19（统一 spawn 原语实现）")
}

// TestSandbox_Adversarial_Windows_DLLHijack — DLL 劫持四件套（R-1089）。
// 断言：SetDefaultDllDirectories+只读执行目录+writable∩dll-search=∅+加载路径审计。
func TestSandbox_Adversarial_Windows_DLLHijack(t *testing.T) {
	t.Skip("先红挂起——转绿归任务 3.25（Windows 对抗实现）")
}
