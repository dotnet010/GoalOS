// 契约测试：I4-I5 层级契约（R-1078/R-1079——会议 #193 威胁模型升级）。
//
// 断言来源: R-1078（L4-L5 隔离层级扩展——gVisor systrap 无 KVM 依赖=信创 4.19 可跑 L4；
//   Windows Sandbox .wsb；macOS 诚实标注维持）；R-1079（L5 一次性执行环境——实现推迟
//   v0.4.0 诚实标注）。
//
// 先红状态: 骨架断言未来 API 契约——backend 层级路由未实现。
// 转绿任务: 3.19~3.23（W3-4 后端实现）。

package sandbox_test

import "testing"

// TestSandbox_GVisorTier_DetectAndRoute — gVisor 层级探测与路由（R-1078）。
// 断言：KVM 可用→gVisor runsc 路径；KVM 不可用→systrap 路径；不兼容→seccomp 兜底。
func TestSandbox_GVisorTier_DetectAndRoute(t *testing.T) {
	t.Skip("先红挂起（R-571）——转绿归任务 3.19（gVisor 后端实现）")
}

// TestSandbox_L5Disposable_HonestDeferral — L5 一次性执行环境诚实标注（R-1079）。
// 断言：L5 请求→能力矩阵标注"推迟 v0.4.0"（诚实标注非伪装支持）。
func TestSandbox_L5Disposable_HonestDeferral(t *testing.T) {
	t.Skip("先红挂起——转绿归任务 3.20（I5 规范+接口占位）")
}
