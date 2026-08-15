// Package statefs — 文件系统状态快照（R-1035——statefs 不可变状态树）。
//
// 骨架阶段（R-571 测试先行闸口）：最小 stub——包可编译、测试可运行、断言处 t.Skip。
// 转绿归任务 3.15（statefs 实现）。
package statefs

import "errors"

// ErrNotImplemented — 骨架阶段占位错误（实现归任务 3.15）。
var ErrNotImplemented = errors.New("statefs: not implemented (skeleton stage)")

// FS — statefs 文件系统快照接口（骨架：New() 返回 ErrNotImplemented）。
type FS struct{}

// New — 构造 statefs 实例（骨架：返回 ErrNotImplemented——先红挂起）。
func New() (*FS, error) {
	return nil, ErrNotImplemented
}
