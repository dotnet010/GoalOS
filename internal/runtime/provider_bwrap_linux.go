//go:build linux

// provider_bwrap_linux.go——Linux 受限档 bwrap 引擎（R-1679——会议 #280 续 Jobs 裁决：
// kysec 逆向停止+bwrap 集成+非核心免自研）。bwrap=bubblewrap——Flatpak 核心沙箱
// 执行体（数千行 C 单二进制，全球审计十余年）：userns+bind-mount 遮蔽+pivot_root
// =模式 C 的现成实现。Go 侧仅拼参数调起——无 worker 模型无 socket 回连无递归面
//（agentbox 三害同源全消）。
//
// 隔离语义（双机实机实证 2026-09-08）：
//   --ro-bind / /        =全文件系统只读（写=EROFS 实拒——Precheck fs 探针命中面）
//   --bind ws/tmp rw     =工作区+临时目录可写（WritableRoots 语义）
//   --unshare-all        =全命名空间隔离（含 net——网络全拒；AF_UNIX 文件系统面不受
//                         netns 影响——FD3 broker unix socket 直连面保留）
//   --die-with-parent    =父死子亡（生命周期绞杀=内核托管）
//   --chdir ws           =CWD 显式=workspace
//
// 依赖面（诚实标注）：bwrap 二进制（麒麟 V10 SP1 系统自带 0.4.0=零税；
// Ubuntu 24.04=apt bubblewrap 0.9.0+goalos-bwrap AppArmor profile——
// scripts/gen-apparmor-profile.sh --bwrap 生成）。
package runtime

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/goalos/goalos/internal/fd3"
)

// bwrapProvider Linux 受限档 bwrap 引擎 Provider。
type bwrapProvider struct {
	workspace string
	tmpDir    string
	prepared  bool
	bwrapPath string
	dialFn    fd3.DialFunc
	onDeny    func(endpoint, reason string)
	mu        sync.Mutex
}

// BwrapOption 构造可选项（与 WinAC/模式 B 同族）。
type BwrapOption func(*bwrapProvider)

// WithBwrapDialFunc 注入 broker 拨号面（生产=zone dialer 同源）。
func WithBwrapDialFunc(d fd3.DialFunc) BwrapOption {
	return func(p *bwrapProvider) { p.dialFn = d }
}

// WithBwrapOnDeny 注入 broker 拒绝审计回调。
func WithBwrapOnDeny(fn func(endpoint, reason string)) BwrapOption {
	return func(p *bwrapProvider) { p.onDeny = fn }
}

// NewBwrapProvider 构造（能力不在此断言——Prepare 实证探测）。
func NewBwrapProvider(workspace, tmpDir string, opts ...BwrapOption) Provider {
	p := &bwrapProvider{workspace: workspace, tmpDir: tmpDir}
	for _, opt := range opts {
		opt(p)
	}
	return p
}

func (p *bwrapProvider) Name() string { return "bwrap-linux" }
func (p *bwrapProvider) Tier() string { return "T1" }

func (p *bwrapProvider) State(context.Context) (ProviderState, error) {
	if p.prepared {
		return ProviderPrepared, nil
	}
	return ProviderRegistered, nil
}

// Capabilities 能力快照（bwrap=ns 级隔离——mount ns 文件系统视图+pivot_root，
// 强于模式 B 的 landlock 路径过滤面；无 seccomp 等价物不自称 I3）。
func (p *bwrapProvider) Capabilities(context.Context) (ProviderCapability, error) {
	return ProviderCapability{
		Platform:          "linux-bwrap",
		AchievedIsolation: I2,
		WarmPool:          false,
	}, nil
}

// Prepare 一次性准备：bwrap 二进制探测+实证 spawn（不信静态存在——
// 实机跑通才算数；Ubuntu 24.04=goalos-bwrap profile 前置）。
func (p *bwrapProvider) Prepare(_ context.Context, _ RuntimePlan) error {
	p.mu.Lock()
	defer p.mu.Unlock()
	path, err := exec.LookPath("bwrap")
	if err != nil {
		return fmt.Errorf("%w: bwrap 二进制缺席（麒麟=系统自带/Ubuntu=apt install bubblewrap——受限档本平台不可用）: %w", ErrNoBackend, err)
	}
	// 实证 spawn（不 spawn 不知可用——R-1664 双引擎收敛同纪律）
	probe := exec.Command(path, "--ro-bind", "/", "/", "--unshare-all", "--die-with-parent", "/bin/true")
	if out, err := probe.CombinedOutput(); err != nil {
		return fmt.Errorf("%w: bwrap 实证 spawn 失败: %v——%s（Ubuntu 24.04=需 goalos-bwrap profile 放行 userns 族）", ErrNoBackend, err, strings.TrimSpace(string(out)))
	}
	for _, root := range []string{p.workspace, p.tmpDir} {
		if err := os.MkdirAll(root, 0o755); err != nil {
			return fmt.Errorf("%w: WritableRoot 创建失败 %s: %w", ErrNoBackend, root, err)
		}
	}
	p.bwrapPath = path
	p.prepared = true
	return nil
}

// Acquire 租约（无预建资源——句柄即配置载体+契约端点集登记）。
func (p *bwrapProvider) Acquire(_ context.Context, req LeaseRequest) (RuntimeHandle, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if !p.prepared {
		return nil, fmt.Errorf("%w: Prepare 未调用", ErrNoBackend)
	}
	return &bwrapHandle{
		p: p, state: HandleAcquired, goalID: req.GoalID,
		endpoints: ContractEndpoints(req.Contract),
	}, nil
}

// bwrapHandle bwrap 执行句柄（D-5 定序同构）。
type bwrapHandle struct {
	mu        sync.Mutex
	p         *bwrapProvider
	state     HandleState
	goalID    string
	endpoints []string
	fd3Ln     *fd3.Listener
	fd3Broker *fd3.Broker
}

func (h *bwrapHandle) ID() string { return "bwrap-" + h.goalID }

func (h *bwrapHandle) State() HandleState {
	h.mu.Lock()
	defer h.mu.Unlock()
	return h.state
}

// Start 边界建立（FD3 面=契约声明端点→broker 拉起）。
func (h *bwrapHandle) Start(context.Context) error {
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.state != HandleAcquired {
		return ErrInvalidState
	}
	if len(h.endpoints) > 0 {
		if err := h.bringUpFD3(); err != nil {
			return fmt.Errorf("runtime: FD3 中继拉起失败（fail-closed）: %w", err)
		}
	}
	h.state = HandleReady
	return nil
}

// bringUpFD3 broker 拉起（unix socket 落 tmpDir——bwrap 绑定该路径进沙箱=
// 沙箱内进程直连；映射文件落 workspace=工作负载知悉面）。
func (h *bwrapHandle) bringUpFD3() error {
	ln, err := fd3.Listen(h.p.tmpDir, "GoalOS-"+h.goalID)
	if err != nil {
		return err
	}
	broker := fd3.NewBroker(h.endpoints, h.p.dialFn, h.p.onDeny)
	go broker.Serve(ln)
	mapContent := "sock=" + ln.Name() + "\n"
	for _, ep := range h.endpoints {
		mapContent += "endpoint=" + ep + "\n"
	}
	if err := os.WriteFile(filepath.Join(h.p.workspace, "goalos-fd3-map.txt"), []byte(mapContent), 0644); err != nil {
		ln.Close()
		return fmt.Errorf("fd3 映射文件写入失败: %w", err)
	}
	h.fd3Ln, h.fd3Broker = ln, broker
	return nil
}

// Precheck 边界实证（原生探针经 bwrap 沙箱执行——fail-closed）。
func (h *bwrapHandle) Precheck(ctx context.Context) error {
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.state != HandleReady && h.state != HandleRunning {
		return ErrInvalidState
	}
	home, _ := os.UserHomeDir()
	probePath := filepath.Join(home, ".goalos-precheck-probe")
	code, out := h.execSandboxed(ctx, []string{"__goalos-probe", "write", probePath})
	_ = os.Remove(probePath)
	if code == 0 {
		return fmt.Errorf("runtime: Precheck fs 探针未被拒（写 home 成功=bwrap 边界失效）——out=%q", out)
	}
	code, out = h.execSandboxed(ctx, []string{"__goalos-probe", "dial", "192.0.2.1:80"})
	if code == 0 {
		return fmt.Errorf("runtime: Precheck 网络探针未被拒（出站成功=bwrap 网络禁闭失效）——out=%q", out)
	}
	h.state = HandleRunning
	return nil
}

// Execute 边界内执行（process.exec 唯一动词）。
func (h *bwrapHandle) Execute(ctx context.Context, req ExecuteRequest) (ExecuteResult, error) {
	h.mu.Lock()
	defer h.mu.Unlock()
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
	start := time.Now()
	code, out := h.execSandboxed(ctx, argv)
	res := ExecuteResult{Output: out, ExitCode: code, Status: "success", Cost: time.Since(start)}
	if code != 0 {
		res.Status = "failed"
	}
	return res, nil
}

// execSandboxed bwrap 沙箱内执行（拼装参数+拉起——一次性进程无 worker 面）。
func (h *bwrapHandle) execSandboxed(ctx context.Context, argv []string) (int, string) {
	self, err := os.Executable()
	if err != nil {
		return -1, "BWRAP-FATAL: " + err.Error()
	}
	args := []string{
		"--ro-bind", "/", "/",
		"--unshare-all", "--unshare-net",
		"--bind", h.p.workspace, h.p.workspace,
		"--bind", h.p.tmpDir, h.p.tmpDir,
		// 注：--dev-bind /dev/null 在麒麟 bwrap 0.4.0 上「Can't create file at
		// /dev/null」实机实锤（老版本对只读根 /dev 节点的创建面残缺）——探针面
		// 无 /dev/null 依赖，不挂（Ubuntu 0.9.0 可挂——版本面差异诚实标注）。
		"--die-with-parent",
		"--chdir", h.p.workspace,
		"--",
		self,
	}
	// argv[0]=binary——探针形态=self re-exec（binary 参数=os.Args[0] 同值）
	// 非探针形态（真实负载二进制）=argv 原样
	if len(argv) > 0 && argv[0] != self {
		args = append(args[:len(args)-1], argv[0]) // 换掉 self=目标二进制
	}
	args = append(args, argv[1:]...)
	cmd := exec.CommandContext(ctx, h.p.bwrapPath, args...)
	out, err := cmd.CombinedOutput()
	code := 0
	if err != nil {
		if ee, ok := err.(*exec.ExitError); ok {
			code = ee.ExitCode()
		} else {
			return -1, "BWRAP-FATAL: " + err.Error()
		}
	}
	return code, string(out)
}

// Release 清理（FD3 监听收尾；无长驻资源——每 Execute=独立进程随退出消亡）。
func (h *bwrapHandle) Release(context.Context) error {
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.state == HandleReleased || h.state == HandleDestroyed {
		return nil
	}
	if h.fd3Ln != nil {
		h.fd3Ln.Close()
	}
	h.state = HandleReleased
	return nil
}

// Interrupt/Pause/Resume——无长驻子进程面（每 Execute 独立进程随 Exit 消亡）。
func (h *bwrapHandle) Interrupt(context.Context) error {
	if h.State() != HandleRunning {
		return ErrInvalidState
	}
	return nil
}

func (h *bwrapHandle) Pause(context.Context) error {
	if h.State() != HandleRunning {
		return ErrInvalidState
	}
	h.state = HandlePaused
	return nil
}

func (h *bwrapHandle) Resume(context.Context) error {
	if h.State() != HandlePaused {
		return ErrInvalidState
	}
	h.state = HandleRunning
	return nil
}
