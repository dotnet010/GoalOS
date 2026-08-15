// 契约测试：fd 卫生+标准最小运行时挂载集（R-1084 扩展 R-1090——会议 #193/#194）。
//
// 断言来源: R-1084（fd CLOEXEC 卫生+标准最小运行时挂载集）；R-1090（§18.4 运行时
//   挂载集机制链三步——ldconfig 快照+cache 哈希/Compile 期冻结/§18.2 挂载子序列）。
//
// 先红状态: fd 卫生验证+挂载集机制未实现（spawn 原语骨架阶段）。
// 转绿任务: 3.19（W3-4 后端实现）。

package sandbox_test

import "testing"

// TestSandbox_Spawn_AllFdsClosedExceptAllowlist — spawn 后子进程 fd 仅 {0,1,2,FD3}（R-1086 ④验证期对抗契约）。
// 断言：spawn 后子进程 /proc/self/fd 枚举=白名单集合。
func TestSandbox_Spawn_AllFdsClosedExceptAllowlist(t *testing.T) {
	t.Skip("先红挂起（R-571）——转绿归任务 3.19（统一 spawn 原语实现）")
}

// TestSandbox_MinimalRuntimeExec — 最小运行时挂载集可执行（R-1090）。
// 断言：最小挂载集（工具链只读 bind 白名单封闭）下目标程序可执行。
func TestSandbox_MinimalRuntimeExec(t *testing.T) {
	t.Skip("先红挂起——转绿归任务 3.19")
}
