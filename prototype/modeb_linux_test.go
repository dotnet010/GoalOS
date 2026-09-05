//go:build prototype && linux

// modeb_linux_test.go——模式 B spike：免 userns 原生沙箱执行器（2026-09-01）
//
// 背景：Ubuntu 24.04 默认 AppArmor 策略（apparmor_restrict_unprivileged_userns=1）
// 阻断 userns 创建→agentbox（namespace 族）fail-open 裸跑（goalos-test 实机实证）。
// 模式 B=PR_SET_NO_NEW_PRIVS+Landlock LSM+seccomp BPF——三者均不需要 userns/root。
//
// 布防（顾问第七~九轮+暗坑四连全采纳）：
//   ②Go 多线程漂移：runtime.LockOSThread()+seccomp TSYNC 标志
//   ①socket/socketpair 按 domain 白名单（仅 AF_UNIX），connect 无法绕过（无 INET
//     socket 可创建）；防未知协议族=白名单非黑名单
//   ③动态链接器面：/usr /lib /lib64 /etc/ld.so.cache /etc/alternatives /proc(ro)
//   ④home 链寻路：祖先目录授 READ_DIR（不含写），~/.ssh 不授予=天然拒
//   P0③符号链接：授权路径先 EvalSymlinks 解析+O_NOFOLLOW 校验解析后路径
//   P0④继承 FD 清理：exec 前关闭 >2 全部 fd
//   P1②架构：amd64 以外 fail-closed（spike 范围——arm64 常量留注记）
//   修正判断：seccomp 全域白名单不采纳（Go runtime syscall 面=跑步机）——本档
//     仅网络族精准过滤；O_PATH|O_NOFOLLOW 拦不住链接（改 O_RDONLY|O_NOFOLLOW）
package prototype

import (
	"fmt"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"syscall"
	"testing"
	"unsafe"
)

// ─── 常量（amd64；arm64=施工期补——P1② fail-closed 纪律：未支持架构不猜） ───
const (
	prSetNoNewPrivs         = 38
	seccompSetModeFilter    = 1
	seccompFilterFlagTSync  = 1
	seccompRetAllow         = 0x7fff0000
	seccompRetErrno         = 0x00050000
	sysSeccomp              = 317
	sysLandlockCreateRuleset = 444
	sysLandlockAddRule       = 445
	sysLandlockRestrictSelf  = 446
	auditArchX8664           = 0xC000003E
	errnoEACCES              = 13

	llExec       = 1 << 0
	llWriteFile  = 1 << 1
	llReadFile   = 1 << 2
	llReadDir    = 1 << 3
	llRemoveDir  = 1 << 4
	llRemoveFile = 1 << 5
	llMakeChar   = 1 << 6
	llMakeDir    = 1 << 7
	llMakeReg    = 1 << 8
	llMakeSock   = 1 << 9
	llMakeFifo   = 1 << 10
	llMakeBlock  = 1 << 11
	llMakeSym    = 1 << 12
	llRefer      = 1 << 13 // ABI v2
	llTruncate   = 1 << 14 // ABI v3
	llMaskV1     = llExec | llWriteFile | llReadFile | llReadDir | llRemoveDir | llRemoveFile |
		llMakeChar | llMakeDir | llMakeReg | llMakeSock | llMakeFifo | llMakeBlock | llMakeSym
)

// TestMain 子命令拦截（re-exec 自身=零外部二进制——__modeb-exec=沙箱施加器+
// __modeb-probe=探针面；单一 TestMain 纪律——prototype 包唯一）。
func TestMain(m *testing.M) {
	if len(os.Args) > 1 {
		switch os.Args[1] {
		case "__modeb-exec":
			modeBExecChild() // 不返回（exec 或 fatal）
		case "__modeb-probe":
			os.Exit(modeBProbeMain())
		}
	}
	os.Exit(m.Run())
}

// llGrant 授权项（path=解析后真实路径）。
type llGrant struct {
	path   string
	access uint64
}

// modeBExecChild 沙箱施加器：landlock→NNP→restrict→seccomp(TSYNC)→清 FD→exec。
// 用法：__modeb-exec <target> [args...]；授权集经环境变量 MODEB_WS/MODEB_TMP/
// MODEB_TOOLCHAIN（冒号分隔）传入。
func modeBExecChild() {
	if len(os.Args) < 3 {
		fmt.Fprintln(os.Stderr, "MODEB-FATAL: 缺 target 参数")
		os.Exit(2)
	}
	if runtime.GOARCH != "amd64" {
		fmt.Fprintf(os.Stderr, "MODEB-FATAL: 未支持架构 %s（fail-closed——P1②）\n", runtime.GOARCH)
		os.Exit(2)
	}
	target := os.Args[2]
	targs := os.Args[2:]

	// ②锁线程——landlock_restrict_self/seccomp 默认仅当前线程
	runtime.LockOSThread()

	// 授权集装配（符号链接解析+O_NOFOLLOW 校验——P0③）
	ws := os.Getenv("MODEB_WS")
	tmpD := os.Getenv("MODEB_TMP")
	selfExe, _ := os.Executable()
	fullRWX := uint64(llMaskV1)
	roX := uint64(llReadFile | llReadDir | llExec)
	rwFile := uint64(llReadFile | llWriteFile)
	grants := []llGrant{{ws, fullRWX}, {tmpD, fullRWX}}
	if selfExe != "" {
		grants = append(grants, llGrant{selfExe, llReadFile | llExec}) // 孙进程自举（TSYNC 探针）
	}
	// 工具链路径（环境传入——R-1659 v3 授权集语义）
	for _, tp := range strings.Split(os.Getenv("MODEB_TOOLCHAIN"), ":") {
		if tp != "" {
			grants = append(grants, llGrant{tp, roX})
		}
	}
	// ③标准最小运行时集（RO）
	for _, p := range []string{"/usr", "/lib", "/lib64", "/etc/ld.so.cache", "/etc/alternatives", "/proc"} {
		grants = append(grants, llGrant{p, roX})
	}
	// /dev 精确单设备（P1①）
	for _, p := range []string{"/dev/null", "/dev/zero", "/dev/urandom"} {
		grants = append(grants, llGrant{p, rwFile})
	}
	// ④home 链祖先=READ_DIR 寻路权（不含写；~/.ssh 不授予=天然拒）
	home := os.Getenv("MODEB_HOME")
	for d := filepath.Dir(ws); d != "/" && d != "." && strings.HasPrefix(d, "/"); d = filepath.Dir(d) {
		grants = append(grants, llGrant{d, llReadDir})
		if d == home || d == filepath.Dir(home) {
			break
		}
	}
	// home 自身保证在链上
	grants = append(grants, llGrant{home, llReadDir})

	ruleset, err := llCreateRuleset()
	if err != nil {
		fmt.Fprintln(os.Stderr, "MODEB-FATAL: ruleset: "+err.Error())
		os.Exit(2)
	}
	for _, g := range grants {
		if err := llAddPathRule(ruleset, g); err != nil {
			fmt.Fprintf(os.Stderr, "MODEB-FATAL: grant %s: %v\n", g.path, err)
			os.Exit(2)
		}
	}
	// NNP 必须在 restrict_self 之前
	if err := prctl(prSetNoNewPrivs, 1); err != nil {
		fmt.Fprintln(os.Stderr, "MODEB-FATAL: no_new_privs: "+err.Error())
		os.Exit(2)
	}
	if err := llRestrictSelf(ruleset); err != nil {
		fmt.Fprintln(os.Stderr, "MODEB-FATAL: restrict_self: "+err.Error())
		os.Exit(2)
	}
	if err := applyNetSeccomp(); err != nil {
		fmt.Fprintln(os.Stderr, "MODEB-FATAL: seccomp: "+err.Error())
		os.Exit(2)
	}
	closeInheritedFDs()
	// exec 目标（边界随 exec 继承——landlock/seccomp 不可撤销）
	env := os.Environ()
	if err := syscall.Exec(target, targs, env); err != nil {
		fmt.Fprintln(os.Stderr, "MODEB-FATAL: exec: "+err.Error())
		os.Exit(2)
	}
}

// ─── Landlock 原语（raw syscall——x/sys/unix 未 vendor，stdlib-only） ───

type llRulesetAttr struct{ handledAccessFS uint64 }
type llPathBeneathAttr struct {
	allowedAccess uint64
	parentFD      int32
	_             int32 // 对齐 16B
}

func llCreateRuleset() (int, error) {
	attr := llRulesetAttr{handledAccessFS: llMaskV1}
	fd, _, e := syscall.Syscall(sysLandlockCreateRuleset,
		uintptr(unsafe.Pointer(&attr)), unsafe.Sizeof(attr), 0)
	if e != 0 {
		return -1, e
	}
	return int(fd), nil
}

// oPath 常量（stdlib syscall 未导出 O_PATH——0o10000000=Linux 定值）。
const oPath = 0o10000000

// llAddPathRule 解析符号链接→O_NOFOLLOW 校验→O_PATH 打开→add_rule。
func llAddPathRule(ruleset int, g llGrant) error {
	real, err := filepath.EvalSymlinks(g.path)
	if err != nil {
		return fmt.Errorf("EvalSymlinks: %w", err)
	}
	// P0③：解析后路径 O_NOFOLLOW 校验（命中链接=ELOOP 熔断）
	chk, err := syscall.Open(real, syscall.O_RDONLY|syscall.O_NOFOLLOW|syscall.O_CLOEXEC, 0)
	if err != nil {
		return fmt.Errorf("nofollow-check %s: %w", real, err)
	}
	syscall.Close(chk)
	fd, err := syscall.Open(real, oPath|syscall.O_CLOEXEC, 0)
	if err != nil {
		return fmt.Errorf("O_PATH %s: %w", real, err)
	}
	defer syscall.Close(fd)
	access := g.access
	// 文件 vs 目录分面：dir-only 权利（READ_DIR/MAKE_*/REMOVE_DIR）施于文件=EINVAL
	//（首红实证：/etc/ld.so.cache 文件被授 roX 含 READ_DIR → invalid argument）
	var st syscall.Stat_t
	if err := syscall.Fstat(fd, &st); err != nil {
		return fmt.Errorf("Fstat %s: %w", real, err)
	}
	if st.Mode&syscall.S_IFDIR == 0 {
		access &= uint64(llExec | llWriteFile | llReadFile | llTruncate)
	}
	attr := llPathBeneathAttr{allowedAccess: access, parentFD: int32(fd)}
	_, _, e := syscall.Syscall6(sysLandlockAddRule, uintptr(ruleset), 1 /*LANDLOCK_RULE_PATH_BENEATH*/,
		uintptr(unsafe.Pointer(&attr)), 0, 0, 0)
	if e != 0 {
		return e
	}
	return nil
}

func llRestrictSelf(ruleset int) error {
	_, _, e := syscall.Syscall(sysLandlockRestrictSelf, uintptr(ruleset), 0, 0)
	if e != 0 {
		return e
	}
	syscall.Close(ruleset)
	return nil
}

func prctl(option, arg2 uintptr) error {
	_, _, e := syscall.Syscall(syscall.SYS_PRCTL, option, arg2, 0)
	if e != 0 {
		return e
	}
	return nil
}

// applyNetSeccomp socket/socketpair domain 白名单（仅 AF_UNIX——暗坑①+九轮补枪）。
// BPF（amd64）：arch 校验→nr==41(socket)/53(socketpair)→args[0]==1(AF_UNIX)?
// ALLOW:ERRNO(EACCES)。TSYNC 强制全线程同步（暗坑②）。
func applyNetSeccomp() error {
	const (
		sysSocket     = 41
		sysSocketpair = 53
		afUnix        = 1
	)
	filters := []syscall.SockFilter{
		{Code: 0x20, Jt: 0, Jf: 0, K: 4},                  // 0: ld arch
		{Code: 0x15, Jt: 0, Jf: 6, K: auditArchX8664},     // 1: arch!=amd64→ERRNO(idx8)
		{Code: 0x20, Jt: 0, Jf: 0, K: 0},                  // 2: ld nr
		{Code: 0x15, Jt: 0, Jf: 1, K: sysSocket},          // 3: socket→4 else→5
		{Code: 0x05, Jt: 0, Jf: 0, K: 1},                  // 4: JA→6（domain 检查）
		{Code: 0x15, Jt: 0, Jf: 3, K: sysSocketpair},      // 5: socketpair→6 else→9 ALLOW
		{Code: 0x20, Jt: 0, Jf: 0, K: 16},                 // 6: ld args[0] low32（domain）
		{Code: 0x15, Jt: 1, Jf: 0, K: afUnix},             // 7: AF_UNIX→9 ALLOW else→8 ERRNO
		{Code: 0x06, Jt: 0, Jf: 0, K: seccompRetErrno | errnoEACCES}, // 8: ERRNO(13)
		{Code: 0x06, Jt: 0, Jf: 0, K: seccompRetAllow},    // 9: ALLOW
	}
	prog := syscall.SockFprog{Len: uint16(len(filters)), Filter: &filters[0]}
	_, _, e := syscall.Syscall(sysSeccomp, seccompSetModeFilter,
		seccompFilterFlagTSync, uintptr(unsafe.Pointer(&prog)))
	if e != 0 {
		return fmt.Errorf("seccomp(TSYNC): %w", e)
	}
	return nil
}

// closeInheritedFDs P0④——exec 前关闭 >2 全部 fd（父进程遗留继承面）。
func closeInheritedFDs() {
	ents, err := os.ReadDir("/proc/self/fd")
	if err != nil {
		return
	}
	for _, ent := range ents {
		var n int
		if _, err := fmt.Sscanf(ent.Name(), "%d", &n); err == nil && n > 2 {
			syscall.Close(n)
		}
	}
}

// ─── 探针面（__modeb-probe——沙箱内运行，ERRNO 数字证据） ───

func probeReport(name string, err error) {
	if err == nil {
		fmt.Printf("PROBE %s LEAK\n", name)
		return
	}
	errno := unwrapErrno(err)
	fmt.Printf("PROBE %s DENIED errno=%d\n", name, errno)
}

func unwrapErrno(err error) int {
	for err != nil {
		if errno, ok := err.(syscall.Errno); ok {
			return int(errno)
		}
		type unwrapper interface{ Unwrap() error }
		u, ok := err.(unwrapper)
		if !ok {
			break
		}
		err = u.Unwrap()
	}
	return -1
}

// modeBProbeMain 探针矩阵（每项=一行 PROBE 输出，父测试解析断言）。
func modeBProbeMain() int {
	ws := os.Getenv("MODEB_WS")
	home := os.Getenv("MODEB_HOME")

	// 1. workspace 写=通（成功向探针——OK=应然，非泄漏）
	if err := os.WriteFile(filepath.Join(ws, "ok.txt"), []byte("x"), 0644); err != nil {
		fmt.Printf("PROBE ws-write DENIED errno=%d\n", unwrapErrno(err))
	} else {
		fmt.Println("PROBE ws-write OK")
	}
	// 2. home 写=拒
	probeReport("home-write", os.WriteFile(filepath.Join(home, ".goalos-modeb-probe"), []byte("x"), 0644))
	os.Remove(filepath.Join(home, ".goalos-modeb-probe")) // 泄漏时清理
	// 3. 敏感目录读=拒（sentinel 由父测试预置）
	probeReport("ssh-read", func() error {
		_, err := os.ReadFile(filepath.Join(home, ".goalos-ssh-sentinel", "secret"))
		return err
	}())
	// 4. AF_INET dial=拒
	probeReport("inet-dial", func() error {
		_, err := syscall.Socket(syscall.AF_INET, syscall.SOCK_STREAM, 0)
		return err
	}())
	// 5. socketpair(AF_INET)=拒（九轮补枪）
	probeReport("socketpair-inet", func() error {
		_, err := syscall.Socketpair(syscall.AF_INET, syscall.SOCK_STREAM, 0)
		return err
	}())
	// 6. AF_UNIX 自建自连=通（白名单生效）
	func() {
		sockPath := filepath.Join(ws, "t.sock")
		ln, err := net.Listen("unix", sockPath)
		if err != nil {
			fmt.Printf("PROBE unix-loop SETUP-FAIL errno=%d\n", unwrapErrno(err))
			return
		}
		defer ln.Close()
		defer os.Remove(sockPath)
		go func() { c, _ := ln.Accept(); if c != nil { c.Close() } }()
		c, err := net.Dial("unix", sockPath)
		if err != nil {
			fmt.Printf("PROBE unix-loop DENIED errno=%d\n", unwrapErrno(err))
			return
		}
		c.Close()
		fmt.Println("PROBE unix-loop OK")
	}()
	// 7. 动态链接二进制（/bin/ls——暗坑③回归锚）
	func() {
		out, err := exec.Command("/bin/ls", ws).CombinedOutput()
		if err != nil {
			fmt.Printf("PROBE dyn-bin DENIED errno=%d out=%q\n", unwrapErrno(err), out)
			return
		}
		fmt.Printf("PROBE dyn-bin OK out=%q\n", strings.TrimSpace(string(out)))
	}()
	// 8. 符号链接逃逸（ELOOP 熔断锚——workspace 内 evil.lnk→/etc/hostname）
	func() {
		link := filepath.Join(ws, "evil.lnk")
		os.Symlink("/etc/hostname", link)
		_, err := os.ReadFile(link)
		probeReport("symlink-escape", err)
		os.Remove(link)
	}()
	// 9. 孙进程同受约束（TSYNC 锚）——沙箱内再 exec 自身写 home
	func() {
		self, _ := os.Executable()
		cmd := exec.Command(self, "__modeb-probe-grandchild")
		cmd.Env = os.Environ()
		out, err := cmd.CombinedOutput()
		if err != nil {
			fmt.Printf("PROBE grandchild LEAK-SETUP-FAIL err=%v out=%q\n", err, out)
			return
		}
		fmt.Printf("PROBE grandchild %s", strings.TrimSpace(string(out)))
	}()
	return 0
}

// 孙进程分支（被 probe 9 拉起——沙箱约束应随 exec 继承）
func init() {
	if len(os.Args) > 1 && os.Args[1] == "__modeb-probe-grandchild" {
		modeBProbeGrandchild()
		os.Exit(0)
	}
}

func modeBProbeGrandchild() {
	home := os.Getenv("MODEB_HOME")
	err := os.WriteFile(filepath.Join(home, ".goalos-modeb-grandchild"), []byte("x"), 0644)
	if err == nil {
		fmt.Println("LEAK")
		os.Remove(filepath.Join(home, ".goalos-modeb-grandchild"))
	} else {
		fmt.Printf("DENIED errno=%d\n", unwrapErrno(err))
	}
}

// ─── 测试驱动 ───

// TestModeB_Spike 模式 B 全探针矩阵（goalos-test 真机=userns 被拦环境）。
// 断言表（数字 errno 证据——本地化文本零依赖）：
//   ws-write=OK / home-write=DENIED 13 / ssh-read=DENIED 13 / inet-dial=DENIED 13 /
//   socketpair-inet=DENIED 13 / unix-loop=OK / dyn-bin=OK / symlink-escape=DENIED 13 /
//   grandchild=DENIED 13（TSYNC+landlock 继承）
func TestModeB_Spike(t *testing.T) {
	self, _ := os.Executable()
	ws := t.TempDir()
	tmpD := t.TempDir()
	home, _ := os.UserHomeDir()

	// 预置：敏感目录 sentinel（父进程态创建——探针读它必须被拒）
	sentinel := filepath.Join(home, ".goalos-ssh-sentinel")
	os.MkdirAll(sentinel, 0700)
	os.WriteFile(filepath.Join(sentinel, "secret"), []byte("s"), 0600)
	defer os.RemoveAll(sentinel)

	cmd := exec.Command(self, "__modeb-exec", self, "__modeb-probe")
	cmd.Env = append(os.Environ(),
		"MODEB_WS="+ws, "MODEB_TMP="+tmpD, "MODEB_HOME="+home,
		"MODEB_TOOLCHAIN="+os.Getenv("GOROOT"),
	)
	out, err := cmd.CombinedOutput()
	text := string(out)
	t.Logf("沙箱输出:\n%s", text)
	if err != nil {
		t.Fatalf("沙箱进程异常退出: %v", err)
	}
	if strings.Contains(text, "MODEB-FATAL") {
		t.Fatalf("沙箱施加失败（fail-closed 路径）: %s", text)
	}

	expect := map[string]string{
		"ws-write":        "OK",
		"home-write":      "DENIED errno=13",
		"ssh-read":        "DENIED errno=13",
		"inet-dial":       "DENIED errno=13",
		"socketpair-inet": "DENIED errno=13",
		"unix-loop":       "OK",
		"symlink-escape":  "DENIED errno=13",
		"grandchild":      "DENIED errno=13",
	}
	for name, want := range expect {
		line := findProbeLine(text, name)
		if line == "" {
			t.Fatalf("探针 %s 无输出（探针面残缺=不可计绿）", name)
		}
		if !strings.Contains(line, want) {
			t.Errorf("探针 %s: 期望 %q 实际 %q", name, want, line)
		}
	}
	// 动态链接二进制=OK（输出含 workspace 内容即 ls 成功）
	dynLine := findProbeLine(text, "dyn-bin")
	if !strings.Contains(dynLine, "OK") {
		t.Errorf("探针 dyn-bin: 期望 OK 实际 %q", dynLine)
	}
	// 泄漏总检：任何 LEAK=红
	for _, line := range strings.Split(text, "\n") {
		if strings.Contains(line, " LEAK") && !strings.Contains(line, "SETUP-FAIL") {
			t.Errorf("CRITICAL 边界泄漏: %s", line)
		}
	}
}

func findProbeLine(out, name string) string {
	for _, line := range strings.Split(out, "\n") {
		if strings.HasPrefix(line, "PROBE "+name+" ") {
			return strings.TrimPrefix(line, "PROBE "+name+" ")
		}
	}
	return ""
}
