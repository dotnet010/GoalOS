// sandbox_doctor.go — goalos sandbox doctor（任务 7.6——信创诊断包 R-929）。
//
// 契约：单命令 tarball——内核信息/特性探测/决策轨迹/contract test 报告/结构化 log/
// panic stack。信创诊断包=一键诊断输出（tarball 打包）。
package daemon

import "errors"

var ErrNotImplemented = errors.New("sandbox doctor: not implemented (skeleton stage)")

// SandboxDoctor — goalos sandbox doctor（信创诊断包）。
// 契约：单命令 tarball——内核信息/特性探测/决策轨迹/contract test 报告/结构化 log/
// panic stack（R-929）。
type SandboxDoctor struct{}

// Diagnose — 信创诊断包（单命令 tarball）。
// 契约：内核信息/特性探测/决策轨迹/contract test 报告/结构化 log/panic stack 打包。
func (d *SandboxDoctor) Diagnose() ([]byte, error) {
	// 骨架：诊断包实现归 7.6 完成态（tarball 打包）。
	return nil, ErrNotImplemented
}
