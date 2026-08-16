// 契约测试：fd 卫生+标准最小运行时挂载集（R-1084 扩展 R-1090——会议 #193/#194）。
//
// 断言来源: R-1084（fd CLOEXEC 卫生+标准最小运行时挂载集）；R-1090（§18.4 运行时
//   挂载集机制链三步——ldconfig 快照+cache 哈希/Compile 期冻结/§18.2 挂载子序列）。
//
// 当前契约形态: 统一 spawn 原语骨架阶段（转绿归 3.19）。本测试断言已落地机制：
//   - profile 文件系统白名单编译（Compile 期冻结的第一步——allow_read+allow_write
//     → CompiledFilesystem.AllowPaths，R-1008 数据流）；
//   - 三层合并语义（R-908：deny 取并集、allow 取交集）——最小挂载集的可执行
//     边界由白名单封闭；
//   - 骨架期后端 Execute fail-closed——spawn 未实现=不伪装子进程执行成功
//     （fd 卫生契约在无 spawn 路径下无泄漏路径）。
//
// 纪律: 行为断言——禁止源码文本断言；逐条 t.Error 非 FailNow。

package sandbox_test

import (
	"context"
	"testing"

	"github.com/goalos/goalos/internal/sandbox"
)

// TestSandbox_Spawn_AllFdsClosedExceptAllowlist — spawn 骨架期 fail-closed 契约
// （R-1086 ④验证期对抗契约的前置形态）。
// 断言：无 spawn 路径=无 fd 泄漏路径（后端 Execute fail-closed——不伪装执行成功）；
// profile 白名单编译正确（Compile 期冻结）。
func TestSandbox_Spawn_AllFdsClosedExceptAllowlist(t *testing.T) {
	raw := &sandbox.RawProfile{
		Isolation: "I2",
		Filesystem: sandbox.FilesystemSection{
			AllowRead:  []string{"/usr/lib", "/tmp/ro"},
			AllowWrite: []string{"/tmp/rw"},
		},
	}
	p, err := sandbox.Compile(raw, sandbox.PlatformLinux)
	if err != nil {
		t.Fatalf("前置: profile 编译失败: %v", err)
	}

	// MUST 1（R-1008 数据流）: allow_read+allow_write → CompiledFilesystem.AllowPaths
	//（Compile 期冻结——挂载集机制第一步）。
	paths := p.Filesystem().AllowPaths
	if len(paths) != 3 {
		t.Errorf("MUST 1（R-1008/R-1090）: AllowPaths 数量=%d，必须为 3（allow_read 2+allow_write 1 冻结）", len(paths))
	}
	seen := map[string]bool{}
	for _, pt := range paths {
		seen[pt] = true
	}
	for _, want := range []string{"/usr/lib", "/tmp/ro", "/tmp/rw"} {
		if !seen[want] {
			t.Errorf("MUST 1（R-1008）: AllowPaths 缺失 %q——白名单编译失真", want)
		}
	}

	// MUST 2（R-1086 前身/R-1468）: 骨架期无 spawn 执行路径——后端 fail-closed
	//（不伪装子进程执行成功=无 fd 泄漏路径）。
	backend := platformBackend(t)
	if _, err := backend.Execute(context.Background(), &sandbox.CommandSpec{Path: "/bin/true"}, *p); err == nil {
		t.Error("MUST 2（R-1086 前身）: 骨架期 Execute 返回 nil error——spawn 未实现但伪装执行成功（fd 卫生契约无载体）")
	}
}

// TestSandbox_MinimalRuntimeExec — 最小运行时挂载集边界（R-1090/R-908）。
// 断言：三层合并语义（deny 取并集、allow 取交集）——挂载集可执行边界由白名单封闭；
// 骨架期执行 fail-closed（最小挂载集运行不伪装成功）。
func TestSandbox_MinimalRuntimeExec(t *testing.T) {
	base := &sandbox.RawProfile{
		Isolation: "I1",
		Filesystem: sandbox.FilesystemSection{
			AllowRead:  []string{"/usr/lib", "/opt/toolchain"},
			AllowWrite: []string{"/tmp"},
			Deny:       []string{"/etc/passwd"},
		},
	}
	user := &sandbox.RawProfile{
		Filesystem: sandbox.FilesystemSection{
			Deny: []string{"/var/log"},
		},
	}

	// MUST 1（R-908 deny 并集）: 任一层的 deny 路径=最终 deny（挂载集封闭下界）。
	merged := sandbox.Merge(base, user, nil)
	denySet := map[string]bool{}
	for _, d := range merged.Filesystem.Deny {
		denySet[d] = true
	}
	for _, want := range []string{"/etc/passwd", "/var/log"} {
		if !denySet[want] {
			t.Errorf("MUST 1（R-908）: 合并后 deny 缺失 %q——deny 并集语义失效", want)
		}
	}
	// MUST 2（R-908 allow 保留）: base 层 allow 在合并后保留（最小挂载集工具链只读白名单）。
	readSet := map[string]bool{}
	for _, p := range merged.Filesystem.AllowRead {
		readSet[p] = true
	}
	for _, want := range []string{"/usr/lib", "/opt/toolchain"} {
		if !readSet[want] {
			t.Errorf("MUST 2（R-908）: 合并后 allow_read 缺失 %q——最小挂载集白名单失真", want)
		}
	}

	// MUST 3（R-1090/R-1468）: 最小挂载集运行骨架 fail-closed——不伪装可执行。
	compiled, err := sandbox.Compile(merged, sandbox.PlatformLinux)
	if err != nil {
		t.Fatalf("前置: 合并 profile 编译失败: %v", err)
	}
	backend := platformBackend(t)
	if _, err := backend.Execute(context.Background(), &sandbox.CommandSpec{Path: "/usr/bin/gcc"}, *compiled); err == nil {
		t.Error("MUST 3（R-1090）: 最小挂载集运行返回 nil error——骨架期伪装可执行违约")
	}
}
