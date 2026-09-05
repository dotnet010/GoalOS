//go:build linux

// modeb_linux.go——模式 B 生产实现：免 userns 原生沙箱（R-1664——双引擎收敛
// 的 Linux 基座）。spike 证据=scripts/red-evidence/2026-09-01-modeb-linux-green.txt
// （goalos-test Ubuntu 24.04 AppArmor 限 userns 环境九探针全绿）。
//
// 机制：re-exec 自身（__goalos-modeb 标记——零外部二进制），子侧依次：
// LockOSThread（暗坑②——per-thread 语义）→Landlock 规则集（EvalSymlinks+
// O_NOFOLLOW 校验授权路径——P0③）→PR_SET_NO_NEW_PRIVS→restrict_self→
// seccomp TSYNC（socket/socketpair domain 白名单——仅 AF_UNIX；暗坑①+九轮补枪）
// →继承 FD 清理（P0④）→exec 目标（边界随 exec 继承不可撤销）。
//
// 诚实声明（九轮——CVE-2020-15257 族）：模式 B 放行 AF_UNIX=抽象套接字面
// 开放（可直连宿主 X11/D-Bus）。激活时由 daemon 启动日志+用户文档显式声明。
package runtime

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"syscall"
	"unsafe"
)

// modeBMarker 子进程标记（daemon main.go/TestMain 拦截——probe.Marker 同族）。
const modeBMarker = "__goalos-modeb"

// modeBConfig 执行配置。
type modeBConfig struct {
	workspace  string   // 完全读写
	tmpDir     string   // 完全读写
	toolchains []string // 只读+执行（R-1659 v3 授权集——Linux 形态）
	allowNet   bool     // true=不装 seccomp 网络过滤（协作档族——预留）
}

// modeBAvailable 实证式能力探测（不问静态声明——landlock ABI 版本查询+架构）。
func modeBAvailable() bool {
	if runtime.GOARCH != "amd64" { // P1② fail-closed——未支持架构不猜（arm64 常量施工期补）
		return false
	}
	// LANDLOCK_CREATE_RULESET_VERSION=1——查 ABI 不建规则集
	_, abi, e := syscall.Syscall(sysLandlockCreateRuleset, 0, 0, 1)
	return e == 0 && abi >= 1
}

// wrapModeB 将 argv 包装为「沙箱内执行」命令（re-exec 自身+标记+配置环境）。
func wrapModeB(selfExe string, argv []string, cfg modeBConfig) *exec.Cmd {
	args := append([]string{modeBMarker}, argv...)
	cmd := exec.Command(selfExe, args...)
	cmd.Env = append(os.Environ(),
		"MODEB_WS="+cfg.workspace, "MODEB_TMP="+cfg.tmpDir,
		"MODEB_TOOLCHAIN="+strings.Join(cfg.toolchains, ":"),
	)
	return cmd
}

// modeBChildMain 子侧入口（由 daemon main/TestMain 在 marker 命中时调用——不返回）。
func modeBChildMain(args []string) {
	if len(args) < 1 {
		fmt.Fprintln(os.Stderr, "MODEB-FATAL: 缺 target")
		os.Exit(2)
	}
	if runtime.GOARCH != "amd64" {
		fmt.Fprintf(os.Stderr, "MODEB-FATAL: 未支持架构 %s（fail-closed——P1②）\n", runtime.GOARCH)
		os.Exit(2)
	}
	target := args[0]

	runtime.LockOSThread() // 暗坑②——landlock/seccomp=per-thread 语义

	cfg := modeBConfig{
		workspace:  os.Getenv("MODEB_WS"),
		tmpDir:     os.Getenv("MODEB_TMP"),
		toolchains: strings.Split(os.Getenv("MODEB_TOOLCHAIN"), ":"),
	}
	grants := assembleModeBGrants(cfg)

	ruleset, err := llCreateRuleset()
	if err != nil {
		fatalModeB("ruleset", err)
	}
	for _, g := range grants {
		if err := llAddPathRule(ruleset, g); err != nil {
			fatalModeB("grant "+g.path, err)
		}
	}
	if err := prctlNoNewPrivs(); err != nil { // NNP 必须在 restrict_self 之前
		fatalModeB("no_new_privs", err)
	}
	if err := llRestrictSelf(ruleset); err != nil {
		fatalModeB("restrict_self", err)
	}
	if err := applyNetSeccomp(); err != nil {
		fatalModeB("seccomp", err)
	}
	closeInheritedFDs() // P0④
	if err := syscall.Exec(target, args, os.Environ()); err != nil {
		fatalModeB("exec", err)
	}
}

func fatalModeB(what string, err error) {
	fmt.Fprintf(os.Stderr, "MODEB-FATAL: %s: %v\n", what, err)
	os.Exit(2)
}

// assembleModeBGrants 授权集（④home 链 READ_DIR 寻路权+③标准运行时集+
// /dev 精确单设备 P1①+自身二进制 RX（孙进程自举）+工具链 RO）。
func assembleModeBGrants(cfg modeBConfig) []llGrant {
	roX := uint64(llReadFile | llReadDir | llExec)
	rwFile := uint64(llReadFile | llWriteFile)
	grants := []llGrant{{cfg.workspace, llMaskV1}, {cfg.tmpDir, llMaskV1}}
	if selfExe, err := os.Executable(); err == nil && selfExe != "" {
		grants = append(grants, llGrant{selfExe, uint64(llReadFile | llExec)})
	}
	for _, tp := range cfg.toolchains {
		if tp != "" {
			grants = append(grants, llGrant{tp, roX})
		}
	}
	for _, p := range []string{"/usr", "/lib", "/lib64", "/etc/ld.so.cache", "/etc/alternatives", "/proc"} {
		grants = append(grants, llGrant{p, roX})
	}
	for _, p := range []string{"/dev/null", "/dev/zero", "/dev/urandom"} {
		grants = append(grants, llGrant{p, rwFile})
	}
	// ④home 链祖先=READ_DIR 寻路权（workspace 在 home 下时——~/.ssh 不授予=天然拒）
	home, _ := os.UserHomeDir()
	for d := filepath.Dir(cfg.workspace); d != "/" && d != "." && strings.HasPrefix(d, "/"); d = filepath.Dir(d) {
		grants = append(grants, llGrant{d, llReadDir})
		if d == home {
			break
		}
	}
	if home != "" {
		grants = append(grants, llGrant{home, llReadDir})
	}
	return grants
}

// ─── Landlock/seccomp 原语（raw syscall——stdlib-only，x/sys/unix 未 vendor） ───

const (
	prSetNoNewPrivs          = 38
	seccompSetModeFilter     = 1
	seccompFilterFlagTSync   = 1
	seccompRetAllow          = 0x7fff0000
	seccompRetErrno          = 0x00050000
	sysSeccomp               = 317
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

	oPath = 0o10000000 // O_PATH（stdlib 未导出）
)

type llGrant struct {
	path   string
	access uint64
}
type llRulesetAttr struct{ handledAccessFS uint64 }
type llPathBeneathAttr struct {
	allowedAccess uint64
	parentFD      int32
	_             int32
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

// llAddPathRule 符号链接解析+O_NOFOLLOW 校验（P0③——ELOOP=熔断）+文件/目录
// 权利分面（dir-only 权利施于文件=EINVAL——spike 首红实证）。
func llAddPathRule(ruleset int, g llGrant) error {
	real, err := filepath.EvalSymlinks(g.path)
	if err != nil {
		return fmt.Errorf("EvalSymlinks: %w", err)
	}
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
	var st syscall.Stat_t
	if err := syscall.Fstat(fd, &st); err != nil {
		return fmt.Errorf("Fstat %s: %w", real, err)
	}
	if st.Mode&syscall.S_IFDIR == 0 {
		access &= uint64(llExec | llWriteFile | llReadFile | llTruncate)
	}
	attr := llPathBeneathAttr{allowedAccess: access, parentFD: int32(fd)}
	_, _, e := syscall.Syscall6(sysLandlockAddRule, uintptr(ruleset), 1,
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

func prctlNoNewPrivs() error {
	_, _, e := syscall.Syscall(syscall.SYS_PRCTL, prSetNoNewPrivs, 1, 0)
	if e != 0 {
		return e
	}
	return nil
}

// applyNetSeccomp socket/socketpair domain 白名单（仅 AF_UNIX——暗坑①+
// 九轮 socketpair 补枪；connect 无法绕过=无 INET socket 可创建）。
func applyNetSeccomp() error {
	const (
		sysSocket     = 41
		sysSocketpair = 53
		afUnix        = 1
	)
	filters := []syscall.SockFilter{
		{Code: 0x20, Jt: 0, Jf: 0, K: 4},                             // ld arch
		{Code: 0x15, Jt: 0, Jf: 6, K: auditArchX8664},                // arch!=amd64→ERRNO
		{Code: 0x20, Jt: 0, Jf: 0, K: 0},                             // ld nr
		{Code: 0x15, Jt: 0, Jf: 1, K: sysSocket},                     // socket→检查
		{Code: 0x05, Jt: 0, Jf: 0, K: 1},                             // JA→domain 检查
		{Code: 0x15, Jt: 0, Jf: 3, K: sysSocketpair},                 // socketpair→检查 else ALLOW
		{Code: 0x20, Jt: 0, Jf: 0, K: 16},                            // ld args[0] low32
		{Code: 0x15, Jt: 1, Jf: 0, K: afUnix},                        // AF_UNIX→ALLOW else ERRNO
		{Code: 0x06, Jt: 0, Jf: 0, K: seccompRetErrno | errnoEACCES}, // ERRNO(13)
		{Code: 0x06, Jt: 0, Jf: 0, K: seccompRetAllow},               // ALLOW
	}
	prog := syscall.SockFprog{Len: uint16(len(filters)), Filter: &filters[0]}
	_, _, e := syscall.Syscall(sysSeccomp, seccompSetModeFilter,
		seccompFilterFlagTSync, uintptr(unsafe.Pointer(&prog)))
	if e != 0 {
		return fmt.Errorf("seccomp(TSYNC): %w", e)
	}
	return nil
}

// closeInheritedFDs P0④——exec 前关闭 >2 全部继承 fd。
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

// modeBProvider 模式 B Provider（Provider SPI——双引擎收敛的 Linux 免 userns 引擎）。
type modeBProvider struct {
	workspace string
	tmpDir    string
	platform  string
	prepared  bool
}

// NewModeBProvider 构造（能力不在此断言——Prepare 实证探测）。
func NewModeBProvider(workspace, tmpDir string) Provider {
	return &modeBProvider{workspace: workspace, tmpDir: tmpDir, platform: "linux-modeb"}
}

func (p *modeBProvider) Name() string { return "modeb-linux" }
func (p *modeBProvider) Tier() string { return "T1" }

func (p *modeBProvider) Prepare(_ context.Context, _ RuntimePlan) error {
	if !modeBAvailable() {
		return fmt.Errorf("%w: 模式 B 不可用（landlock ABI 缺席或非 amd64——fail-closed）", ErrNoBackend)
	}
	for _, root := range []string{p.workspace, p.tmpDir} {
		if err := os.MkdirAll(root, 0o755); err != nil {
			return fmt.Errorf("%w: WritableRoot 创建失败 %s: %w", ErrNoBackend, root, err)
		}
	}
	p.prepared = true
	return nil
}

func (p *modeBProvider) Capabilities(context.Context) (ProviderCapability, error) {
	return ProviderCapability{
		Platform:          p.platform,
		AchievedIsolation: I2, // fs 双面禁闭+网络禁闭（seccomp 网络族限定——HasSyscallConfine 分列诚实注记）
		WarmPool:          false,
	}, nil
}

// State Provider 状态（registered/prepared 两态——模式 B 无后台资源）。
func (p *modeBProvider) State(context.Context) (ProviderState, error) {
	if p.prepared {
		return ProviderPrepared, nil
	}
	return ProviderRegistered, nil
}

// Acquire 租约（模式 B 无预建资源——句柄即配置载体）。
func (p *modeBProvider) Acquire(_ context.Context, req LeaseRequest) (RuntimeHandle, error) {
	return &modeBHandle{p: p, state: HandleAcquired, goalID: req.GoalID}, nil
}

// modeBHandle 模式 B 执行句柄（D-5 定序同构：Start→Precheck→Execute）。
type modeBHandle struct {
	p      *modeBProvider
	state  HandleState
	goalID string
}

func (h *modeBHandle) Start(context.Context) error {
	if h.state != HandleAcquired {
		return ErrInvalidState
	}
	h.state = HandleReady
	return nil
}

// Precheck 边界实证（原生探针经模式 B 沙箱执行——ERRNO 数字断言，fail-closed）。
func (h *modeBHandle) Precheck(ctx context.Context) error {
	if h.state != HandleReady && h.state != HandleRunning {
		return ErrInvalidState
	}
	home, _ := os.UserHomeDir()
	probePath := filepath.Join(home, ".goalos-precheck-probe")
	code, out := h.execSandboxed(ctx, []string{"__goalos-probe", "write", probePath})
	_ = os.Remove(probePath) // 泄漏时清理（边界缺席=文件已落 home）
	if code == 0 {
		return fmt.Errorf("runtime: Precheck fs 探针未被拒（写 home 成功=模式 B 边界失效）——out=%q", out)
	}
	code, out = h.execSandboxed(ctx, []string{"__goalos-probe", "dial", "192.0.2.1:80"})
	if code == 0 {
		return fmt.Errorf("runtime: Precheck 网络探针未被拒（出站成功=NetworkBlocked 失效）——out=%q", out)
	}
	h.state = HandleRunning
	return nil
}

// Execute 边界内执行（process.exec 唯一动词——分类器透传=治理归 GoalOS）。
func (h *modeBHandle) Execute(ctx context.Context, req ExecuteRequest) (ExecuteResult, error) {
	if h.state != HandleRunning {
		return ExecuteResult{}, ErrInvalidState
	}
	if req.ActionType != "process.exec" {
		return ExecuteResult{}, fmt.Errorf("runtime: 受限档不支持的能力动词 %q（process.exec 唯一）", req.ActionType)
	}
	binary := req.Params["binary"]
	if binary == "" {
		return ExecuteResult{}, fmt.Errorf("runtime: process.exec 缺 binary 参数（代理层拒绝——未触 OS 边界）")
	}
	argv := append([]string{binary}, strings.Fields(req.Params["args"])...)
	code, out := h.execSandboxed(ctx, argv)
	res := ExecuteResult{Output: out, ExitCode: code, Status: "success"}
	if code != 0 {
		res.Status = "failed"
	}
	return res, nil
}

// execSandboxed 模式 B 沙箱内执行（re-exec 自身+标记；输出捕获）。
func (h *modeBHandle) execSandboxed(_ context.Context, argv []string) (int, string) {
	self, err := os.Executable()
	if err != nil {
		return -1, "MODEB-FATAL: " + err.Error()
	}
	cmd := wrapModeB(self, argv, modeBConfig{workspace: h.p.workspace, tmpDir: h.p.tmpDir})
	out, err := cmd.CombinedOutput()
	code := 0
	if err != nil {
		if ee, ok := err.(*exec.ExitError); ok {
			code = ee.ExitCode()
		} else {
			return -1, "MODEB-FATAL: " + err.Error()
		}
	}
	return code, string(out)
}

func (h *modeBHandle) Release(context.Context) error {
	h.state = HandleReleased
	return nil
}

func (h *modeBHandle) ID() string { return "modeb-" + h.goalID }

// Interrupt/Pause/Resume——模式 B 无长驻子进程面（每次 Execute=独立沙箱进程，
// 随 Exit 消亡），中断=无对象。按契约表语义：Running 外=ErrInvalidState，
// Running 内=幂等成功（无子进程可中断=目标态已达）。
func (h *modeBHandle) Interrupt(context.Context) error {
	if h.state != HandleRunning {
		return ErrInvalidState
	}
	return nil
}

func (h *modeBHandle) Pause(context.Context) error {
	if h.state != HandleRunning {
		return ErrInvalidState
	}
	h.state = HandlePaused
	return nil
}

func (h *modeBHandle) Resume(context.Context) error {
	if h.state != HandlePaused {
		return ErrInvalidState
	}
	h.state = HandleRunning
	return nil
}

// State 句柄状态（全态可调——契约表）。
func (h *modeBHandle) State() HandleState { return h.state }
