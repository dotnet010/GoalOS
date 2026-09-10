//go:build darwin

// seatbelt_goruntime_contract_test.go——受限档 profile Go 运行时引导面回归防线
// （2026-09-10 物理机实证事故——RED→GREEN 对=red-evidence/2026-09-10-fd3-darwin-sunpath-*.txt
// 同窗口）：R-1666 原生探针/FD3 探针/生产插件载体=Go 二进制，deny default 下 sysctl-read
// 全拒 → Go 进程启动即 panic「failed to get system page size」（raw sysctl MIB 形态——
// sysctl-name 精确名不命中，Darwin 24.5 差分实证；sysctl-name-prefix "hw." 命中）。
// 本测试=内容断言+实证执行双层（同 TC-RT-051 风格）：删除/漂移该放行=darwin 红回归。
package sandbox

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// TestSeatbelt_GoRuntimeBootAllowance：
// ①基础受限 profile 含 hw. 前缀放行（Go 引导面在位）；②sysctl-write 仍全拒（危险面不动）；
// ③绝不含裸 (allow sysctl-read)（收窄纪律——前缀形态在位，防全量放行漂移）；
// ④实证：profile 物化+sandbox-exec 内跑 Go 二进制（自身测试二进制）=启动成功
//
//	（反虚假绿：execvp 级 "sandbox-exec:" 前缀=profile 未生效，不计——R-1641 纪律）。
func TestSeatbelt_GoRuntimeBootAllowance(t *testing.T) {
	p := RestrictedSeatbeltProfile()
	if !strings.Contains(p, `(allow sysctl-read (sysctl-name-prefix "hw."))`) {
		t.Fatal("①Go 引导面缺席：受限 profile 应含 (allow sysctl-read (sysctl-name-prefix \"hw.\"))——删除即 darwin Go 载体红回归")
	}
	if !strings.Contains(p, "(deny sysctl-write)") {
		t.Fatal("②sysctl-write 拒绝面失守")
	}
	if strings.Contains(p, "(allow sysctl-read)") {
		t.Fatal("③收窄纪律失守：绝不含裸 (allow sysctl-read) 全量放行")
	}

	// ④实证执行（仅非 race 构建）：TSan 载体在受限 Seatbelt 内 CHECK failed
	// （sanitizer_mac.cpp——边界拒绝 TSan 运行时所需面=fail-closed 实证，非边界失效；
	// darwin+race 实证面=登记缺口，内容断言①②③在所有构建面在位）
	if raceInstrumented {
		t.Log("④实证步跳过：race 构建（TSan 载体⊥受限 Seatbelt——fail-closed 实证）")
		return
	}
	// 物化 profile+三参数注入（生产同源形态），边界内跑 Go 二进制
	dir := t.TempDir()
	profPath := filepath.Join(dir, "restricted.sb")
	if err := os.WriteFile(profPath, []byte(p), 0600); err != nil {
		t.Fatal(err)
	}
	self, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	if c, err := filepath.EvalSymlinks(self); err == nil { // firmlink 规范化（生产同源）
		self = c
	}
	home, _ := os.UserHomeDir()
	out, err := exec.Command("/usr/bin/sandbox-exec",
		"-f", profPath,
		"-D", "WORKSPACE_DIR="+dir, "-D", "TMP_DIR="+dir, "-D", "HOME_DIR="+home,
		"-D", "TARGET_BINARY="+self,
		"--", self, "-test.run=^$").CombinedOutput()
	if strings.Contains(string(out), "sandbox-exec:") {
		t.Fatalf("④execvp 级失败（profile 未生效伪证——不计）: %q", out)
	}
	if err != nil || strings.Contains(string(out), "failed to get system page size") {
		t.Fatalf("④Go 二进制边界内启动失败（引导面回归）——err=%v out=%q", err, out)
	}
}
