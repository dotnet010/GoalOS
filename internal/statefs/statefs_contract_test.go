// 契约测试：statefs 快照隔离（R-1035/R-1062/R-1065/R-1066——会议 #179/#191/#192）。
//
// 断言来源: R-1035（statefs 不可变状态树入 v0.3.0——Snapshot/Commit 原子切换/Discard/
//   历史只读/崩溃双对账）；R-1062（对账约束升级 K8s level-based reconciliation）；
//   R-1065（§3.1 快照策略分层契约——reflink 优先/禁 live 硬链接/写路径 write-to-temp+
//   rename）；R-1066（Windows Commit 两段指针切换+意图日志幂等回放）。
//
// 当前契约形态（R-1366 修订，会议 #202~#204）: 文件级回滚挂 v0.4（C-PLAT-09
//   Jobs/Meyer 批准）——本包处于骨架期。骨架期契约=R-1468/R-1473 拍板形态：
//   New() 显式返回 ErrNotImplemented（fail-closed——不伪装成功、不暴露伪 API）。
//   3.15 转绿实现时本测试断言升级为快照隔离/对账幂等/原子切换的完整行为。
//
// 纪律: 编译安全探针+行为断言——禁止源码文本断言；逐条 t.Error 非 FailNow。

package statefs_test

import (
	"errors"
	"reflect"
	"testing"

	"github.com/goalos/goalos/internal/statefs"
)

// TestStatefs_Snapshot_Isolates — 骨架期 fail-closed 契约（R-1366/R-1468）。
// 断言：New() 必须返回错误（不伪装成功）+ fs 必须为 nil（不产生部分实例）+
// 错误必须为显式 ErrNotImplemented 拍板形态（R-1473——非临时文本）。
func TestStatefs_Snapshot_Isolates(t *testing.T) {
	fs, err := statefs.New()
	if err == nil {
		t.Error("MUST（R-1366/R-1468）: 骨架期 New() 必须返回错误——伪装成功=fail-open 违约")
	}
	if fs != nil {
		t.Error("MUST（R-1366）: 骨架期 New() 不得返回非 nil 实例——不产生部分状态")
	}
	if !errors.Is(err, statefs.ErrNotImplemented) {
		t.Errorf("MUST（R-1473）: 骨架期错误必须为显式 ErrNotImplemented 拍板形态——errors.Is 判定失败: %v", err)
	}
}

// TestStatefs_Reconcile_Idempotent — 骨架期确定性 fail-closed（R-1062 前身契约）。
// 断言：重复 New() 每次返回相同形态错误且实例恒 nil——对账/构造无状态残留
// （转绿后升级为：同一目标态重复对账=状态不变+审计 no-op）。
func TestStatefs_Reconcile_Idempotent(t *testing.T) {
	fs1, err1 := statefs.New()
	fs2, err2 := statefs.New()
	if err1 == nil || err2 == nil {
		t.Error("MUST（R-1062 前身）: 重复 New() 必须持续 fail-closed——骨架期无成功路径")
	}
	if fs1 != nil || fs2 != nil {
		t.Error("MUST（R-1062 前身）: 重复 New() 实例必须恒 nil——无状态残留")
	}
	if !errors.Is(err1, statefs.ErrNotImplemented) || !errors.Is(err2, statefs.ErrNotImplemented) {
		t.Errorf("MUST（R-1062 前身）: 重复 New() 错误形态必须一致（ErrNotImplemented）——err1=%v err2=%v", err1, err2)
	}
}

// TestStatefs_Commit_AtomicSwitch — 骨架期不暴露伪 API（R-1035 前身契约）。
// 断言：FS 公开方法数必须为 0——骨架期不得暴露 Snapshot/Commit 假接口
// （假接口=调用方可拿到"成功"假象——R-1468 骨架不伪装成功）。
func TestStatefs_Commit_AtomicSwitch(t *testing.T) {
	typ := reflect.TypeOf(statefs.FS{})
	if typ.NumMethod() != 0 {
		t.Errorf("MUST（R-1035 前身/R-1468）: 骨架期 FS 公开方法数=%d，必须为 0——不得暴露伪 API", typ.NumMethod())
	}
	if _, err := statefs.New(); err == nil {
		t.Error("MUST（R-1035 前身）: Commit 路径入口（New）必须 fail-closed——无静默成功")
	}
}
