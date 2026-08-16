// fd_hygiene.go — fd CLOEXEC 卫生审计（任务 7.3——全量跨后端收敛 R-1074）。
//
// 契约（R-928/R-1086 四防线）：①创建期 O_CLOEXEC CI AST ②启动期 close_range+4.19
// /proc 兜底（CPython bpo-47260 无条件 fallback 教训）③spawn 期唯一入口 ④验证期
// 对抗契约（子进程 fd 仅 {0,1,2,FD3}）。本轮=pluginrunner 全路径逐 fd 审计+跨后端统一。
package pluginrunner

import "errors"

var ErrNotImplemented = errors.New("pluginrunner: not implemented (skeleton stage)")

// FdHygieneAudit — fd CLOEXEC 卫生审计（全量跨后端收敛）。
// 契约：pluginrunner 全路径逐 fd 审计+跨后端统一（Unix fd 继承收口/Windows HANDLE_LIST
// 白名单——R-1086 同一原语）。
type FdHygieneAudit struct{}

// Audit — fd 卫生审计（全路径逐 fd）。
// 契约：spawn 期唯一入口+子进程 fd 仅 {0,1,2,FD3}（验证期对抗契约 R-1086 ④）。
func (f *FdHygieneAudit) Audit() error {
	// 骨架：全路径逐 fd 审计实现归 7.3 完成态。
	return ErrNotImplemented
}
