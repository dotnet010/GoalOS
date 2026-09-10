//go:build windows

// provider_winac_windows_test.go——Windows AppContainer Provider 契约测试
//（R-1648/R-1661 v2/R-1659 v3/R-1667 v2/R-1669/R-1670——R-571 测试先行：先红后绿）。
//
// 契约面（决议逐字映射）：
//  A. 边界矩阵：workspace 写=通；home 写=拒；~/.ssh 读=拒；出站=拒；
//     ERRNO 数字证据（R-1666）+Win32 原生常量绑定（R-1670——禁魔数字面量）。
//  B. 具名能力粒度（R-1659 v3）：契约声明 toolchain=读通；未声明=拒。
//  C. 生命周期（R-1661 v2）：Release=Job 绞杀（长活子进程被杀）+重复 Release 幂等。
//     R-1669 tainted fail-safe=内核态 wedge 不可机器复现——评审级覆盖（诚实缺口登记）。
//  D. D-5 定序：Precheck 前 Execute=ErrInvalidState；Start 前 Precheck=ErrInvalidState。
//  E. 熵化命名（R-1667 v2）：两次 Acquire=异名+格式 GoalOS-AC-<action≤24>-<16hex>。
package runtime

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/goalos/goalos/internal/governance"
	"golang.org/x/sys/windows"
)

// winACTestProvider 测试构造（临时目录隔离——不碰真实 home\Goals）。
func winACTestProvider(t *testing.T, toolchains map[string]string) (Provider, string) {
	t.Helper()
	ws := t.TempDir()
	return NewWinACProvider(ws, t.TempDir(), toolchains), ws
}

// winACContract 测试契约（同包构造 VerifiedContract——capabilities 可配；
// TokenClaims 时间戳=int64 Unix 秒）。
func winACContract(goalID, actionID string, caps []string) *VerifiedContract {
	return &VerifiedContract{claims: governance.TokenClaims{
		GoalID: goalID, ActionID: actionID, Capabilities: caps,
		IssuedAt: time.Now().Unix(), ExpiresAt: time.Now().Add(time.Hour).Unix(),
	}}
}

// execProbe 探针执行（契约测试通用面——self+__goalos-probe 子命令）。
func execProbe(t *testing.T, guard *HandleGuard, args string) ExecuteResult {
	t.Helper()
	self, err := os.Executable()
	if err != nil {
		t.Fatalf("os.Executable: %v", err)
	}
	res, err := guard.Execute(context.Background(), ExecuteRequest{
		ActionID: "probe", ActionType: "process.exec",
		Params: map[string]string{"binary": self, "args": args},
	})
	if err != nil {
		t.Fatalf("probe Execute 失败: %v", err)
	}
	return res
}

// errnoOf 提取探针输出的 ERRNO 数字（R-1666 证据形态——-999=缺席）。
func errnoOf(t *testing.T, out string) int {
	t.Helper()
	for _, line := range strings.Split(out, "\n") {
		if strings.HasPrefix(line, "PROBE-ERRNO=") {
			var v int
			if _, err := fmt.Sscanf(line, "PROBE-ERRNO=%d", &v); err == nil {
				return v
			}
		}
	}
	return -999
}

// winACReady Prepare→Acquire→Start→Precheck 全链（D-5 定序）。
func winACReady(t *testing.T, p Provider, goalID, actionID string, caps []string) *HandleGuard {
	t.Helper()
	ctx := context.Background()
	if err := p.Prepare(ctx, RuntimePlan{PlanID: goalID, Tier: TierRestricted}); err != nil {
		t.Fatalf("Prepare 失败: %v", err)
	}
	h, err := p.Acquire(ctx, LeaseRequest{GoalID: goalID, ActionID: actionID,
		Contract: winACContract(goalID, actionID, caps)})
	if err != nil {
		t.Fatalf("Acquire: %v", err)
	}
	guard := NewHandleGuard(h)
	if err := guard.Start(ctx); err != nil {
		t.Fatalf("Start: %v", err)
	}
	if err := guard.Precheck(ctx); err != nil {
		t.Fatalf("Precheck: %v", err)
	}
	return guard
}

// TestWinACProvider_Boundary 边界矩阵（A 面——受限档契约逐字）。
func TestWinACProvider_Boundary(t *testing.T) {
	p, ws := winACTestProvider(t, nil)
	guard := winACReady(t, p, "winac-b", "boundary", []string{"shell.execute"})
	defer guard.Release(context.Background())

	home, _ := os.UserHomeDir()
	// (1)home 写=拒（断言绑定 Win32 原生常量——R-1670：禁魔数字面量/POSIX 误读）
	res := execProbe(t, guard, "__goalos-probe write "+filepath.Join(home, "goalos-winac-probe.txt"))
	if errnoOf(t, res.Output) != int(windows.ERROR_ACCESS_DENIED) {
		t.Fatalf("(1)home 写未按预期拒绝（应 ERRNO=ERROR_ACCESS_DENIED）: out=%q errno=%d", res.Output, errnoOf(t, res.Output))
	}
	// (2)~/.ssh 读=拒（夹具纪律——2026-09-06 事故实证：严禁覆盖真实用户文件！
	// 曾直接写 ~/.ssh/config 致用户 SSH 配置毁损；夹具=新建专用名文件
	// （绝不触碰 config/id_rsa 等真实条目——断言语义不变：AC token 读 .ssh 内文件=拒），用后删除）
	sshDir := filepath.Join(home, ".ssh")
	if err := os.MkdirAll(sshDir, 0700); err != nil {
		t.Fatal(err)
	}
	fixture := filepath.Join(sshDir, "goalos-winac-readfixture")
	if err := os.WriteFile(fixture, []byte("probe"), 0600); err != nil {
		t.Fatal(err)
	}
	defer os.Remove(fixture)
	res = execProbe(t, guard, "__goalos-probe read "+fixture)
	if errnoOf(t, res.Output) != int(windows.ERROR_ACCESS_DENIED) {
		t.Fatalf("(2)~/.ssh 读未拒（errno 应=ERROR_ACCESS_DENIED）: out=%q errno=%d", res.Output, errnoOf(t, res.Output))
	}
	// (3)出站=拒（零 capability AppContainer——WSAEACCES 常量绑定 R-1670）
	res = execProbe(t, guard, "__goalos-probe dial 192.0.2.1:80")
	if errnoOf(t, res.Output) == 0 {
		t.Fatalf("(3)CRITICAL：出站成功——网络禁闭失效: %q", res.Output)
	}
	if errnoOf(t, res.Output) != int(windows.WSAEACCES) {
		t.Fatalf("(3)出站拒绝但 errno 非 WSAEACCES: errno=%d out=%q", errnoOf(t, res.Output), res.Output)
	}
	// (4)workspace 写=通（WritableRoots 语义）
	res = execProbe(t, guard, "__goalos-probe write "+filepath.Join(ws, "ok.txt"))
	if errnoOf(t, res.Output) != 0 {
		t.Fatalf("(4)workspace 写被拒（WritableRoots 语义失守）: errno=%d out=%q", errnoOf(t, res.Output), res.Output)
	}
}

// TestWinACProvider_ToolchainGranularity 具名能力粒度（B 面——R-1659 v3）。
func TestWinACProvider_ToolchainGranularity(t *testing.T) {
	toolDir := t.TempDir()
	toolData := filepath.Join(toolDir, "data.txt")
	if err := os.WriteFile(toolData, []byte("payload"), 0644); err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	p := NewWinACProvider(t.TempDir(), t.TempDir(), map[string]string{"pytool": toolDir})
	if err := p.Prepare(ctx, RuntimePlan{PlanID: "winac-gran", Tier: TierRestricted}); err != nil {
		t.Fatalf("Prepare: %v", err)
	}

	// 未声明 toolchain 能力 → 读=拒
	guard := winACReady(t, p, "g", "nope", []string{"shell.execute"})
	res := execProbe(t, guard, "__goalos-probe read "+toolData)
	if errnoOf(t, res.Output) == 0 {
		t.Fatalf("B1 CRITICAL：未声明 toolchain 竟可读——粒度失守")
	}
	guard.Release(ctx)

	// 声明 toolchain:pytool → 读=通
	guard2 := winACReady(t, p, "g", "yes", []string{"shell.execute", "toolchain:pytool"})
	defer guard2.Release(ctx)
	res = execProbe(t, guard2, "__goalos-probe read "+toolData)
	if errnoOf(t, res.Output) != 0 {
		t.Fatalf("B2：声明后读仍拒（粒度授予失效）: errno=%d out=%q", errnoOf(t, res.Output), res.Output)
	}
}

// TestWinACProvider_Lifecycle 生命周期（C 面——R-1661 v2）。
func TestWinACProvider_Lifecycle(t *testing.T) {
	p, _ := winACTestProvider(t, nil)
	ctx := context.Background()
	guard := winACReady(t, p, "winac-life", "l", []string{"shell.execute"})

	// 长活子进程（sleep 60s）后台执行→Release 应绞杀（Job KILL_ON_JOB_CLOSE）
	done := make(chan struct{})
	go func() {
		defer close(done)
		self, _ := os.Executable()
		guard.Execute(ctx, ExecuteRequest{ActionID: "sleeper", ActionType: "process.exec",
			Params: map[string]string{"binary": self, "args": "__goalos-probe sleep 60000"}})
	}()
	time.Sleep(500 * time.Millisecond) // 让子进程真实拉起
	start := time.Now()
	if err := guard.Release(ctx); err != nil {
		t.Fatalf("Release: %v", err)
	}
	select {
	case <-done:
		if time.Since(start) > 10*time.Second {
			t.Fatalf("C1：子进程绞杀超时（Job KILL_ON_JOB_CLOSE 未生效）")
		}
	case <-time.After(15 * time.Second):
		t.Fatal("C1 CRITICAL：Release 后子进程未死（15s 超时）")
	}
	// 重复 Release=幂等
	if err := guard.Release(ctx); err != nil {
		t.Fatalf("C2：重复 Release 非幂等: %v", err)
	}
}

// TestWinACProvider_D5Ordering D-5 定序（D 面——契约表守门）。
func TestWinACProvider_D5Ordering(t *testing.T) {
	p, _ := winACTestProvider(t, nil)
	ctx := context.Background()
	if err := p.Prepare(ctx, RuntimePlan{PlanID: "winac-d5", Tier: TierRestricted}); err != nil {
		t.Fatalf("Prepare: %v", err)
	}
	h, err := p.Acquire(ctx, LeaseRequest{GoalID: "winac-d5", ActionID: "d"})
	if err != nil {
		t.Fatalf("Acquire: %v", err)
	}
	// Start 前 Precheck=ErrInvalidState
	if err := h.Precheck(ctx); err != ErrInvalidState {
		t.Fatalf("D1：Start 前 Precheck 应 ErrInvalidState，实际 %v", err)
	}
	// Precheck 前 Execute=ErrInvalidState
	if _, err := h.Execute(ctx, ExecuteRequest{ActionID: "x", ActionType: "process.exec",
		Params: map[string]string{"binary": "cmd.exe"}}); err != ErrInvalidState {
		t.Fatalf("D2：Running 前 Execute 应 ErrInvalidState，实际 %v", err)
	}
}

// TestWinACProvider_ProfileEntropy 熵化命名契约（R-1667 v2——会议 #269）：
// 两次 Acquire=两个不同 profile 名且格式=GoalOS-AC-<action≤24>-<16hex 熵>。
// 撞名重试路径=构造性免证（熵空间 2^64，机器不可强制撞名——诚实标注）。
// tainted 路径（R-1669 内核态 wedge）同理不可机器复现——代码评审级覆盖，
// 实机复现需驱动级 wedge 注入（登记诚实缺口，不虚报绿）。
func TestWinACProvider_ProfileEntropy(t *testing.T) {
	p, _ := winACTestProvider(t, nil)
	ctx := context.Background()
	if err := p.Prepare(ctx, RuntimePlan{PlanID: "winac-ent", Tier: TierRestricted}); err != nil {
		t.Fatalf("Prepare: %v", err)
	}
	acquire := func(action string) *winACHandle {
		h, err := p.Acquire(ctx, LeaseRequest{GoalID: "g", ActionID: action,
			Contract: winACContract("g", action, []string{"shell.execute"})})
		if err != nil {
			t.Fatalf("Acquire %s: %v", action, err)
		}
		return h.(*winACHandle)
	}
	h1 := acquire("entropy-a")
	defer h1.Release(ctx)
	h2 := acquire("entropy-a")
	defer h2.Release(ctx)
	if h1.profile == h2.profile {
		t.Fatalf("两次 Acquire profile 同名=%q——熵化命名失效（R-1667 v2）", h1.profile)
	}
	nameRe := regexp.MustCompile(`^GoalOS-AC-[A-Za-z0-9_-]{1,24}-[0-9a-f]{16}$`)
	for _, n := range []string{h1.profile, h2.profile} {
		if !nameRe.MatchString(n) {
			t.Fatalf("profile 名 %q 不符熵化格式（GoalOS-AC-<action≤24>-<16hex>）", n)
		}
	}
	t.Logf("熵名样本: %q / %q", h1.profile, h2.profile)
}
