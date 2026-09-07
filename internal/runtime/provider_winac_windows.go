//go:build windows

// provider_winac_windows.go——Windows AppContainer 受限档 Provider（R-1648——
// Windows 受限档基座；spike 证据链=scripts/red-evidence/2026-08-3*~09-01 族）。
//
// 决议落地映射：
//   R-1661 v2 生命周期：session profile（熵化一次性名）+Job KILL_ON_JOB_CLOSE
//     （内核级绞杀不依赖 daemon 存活）+DeleteAppContainerProfile 退避重试
//     （上限 10 次）+重复 Release 幂等。
//   R-1667 v2 命名纪律（会议 #269 Jobs 裁决）：profile 名=GoalOS-AC-<action 前缀>
//     -<crypto/rand 64bit 熵>——一次性资源（同名重建永久损坏实机纪律）；
//     ALREADY_EXISTS=换名重试×3（严禁 derive 复用继承残留态）；其余失败=fail-closed
//     带真实 HRESULT（pszDescription 不可为 NULL——幻影 profile 实锤归因）。
//   R-1669 超时处决 fail-safe（会议 #269 Jobs 裁决）：execInContainer 超时路径
//     TerminateProcess 后 2s bracket 确认物理死亡；未死=卡入不可中断内核态→
//     tainted 标记→Release 焊死 FreeSid/DeleteProfile（宁泄露不 UAF）。
//   R-1659 v3 具名能力：DeriveCapabilitySidsFromName("GoalOS-TC-<name>") 纯名称派生
//     （零 profile 实体零残留）——工具链目录 RX 一次授予（幂等）；session 容器
//     Capabilities[] 按契约声明携带（SE_GROUP_ENABLED——=0 静默无效实机实锤）。
//     写面=session 包 SID 粒度（workspace 授予 M，Release 回收）。
//   R-1653：执行≠读分离（启动面=CreateProcess 父 token；运行期=AC token）。
//   F6：子进程 CWD 显式=workspace（继承 daemon CWD 不可读=「当前目录无效」实机实证）。
//   R-1666：Precheck=原生探针（__goalos-probe——ERRNO 数字证据）。
//   R-1668（会议 #268）：锁只护 state——执行体与绞杀全在锁外+wg 在飞计数
//     （Execute 全程持锁=Release 绞杀死锁实机实锤）。
package runtime

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"syscall"
	"time"
	"unsafe"

	"golang.org/x/sys/windows"

	"github.com/goalos/goalos/internal/fd3"
)

// ─── Win32 原语（userenv/kernelbase/advapi32——spike 实证归属） ───

var (
	winACUserenv    = windows.NewLazySystemDLL("userenv.dll")
	winACKernelbase = windows.NewLazySystemDLL("kernelbase.dll")
	winACAdvapi     = windows.NewLazySystemDLL("advapi32.dll")

	procCreateAppContainerProfile     = winACUserenv.NewProc("CreateAppContainerProfile")
	procDeleteAppContainerProfile     = winACUserenv.NewProc("DeleteAppContainerProfile")
	procDeriveCapabilitySidsFromName  = winACKernelbase.NewProc("DeriveCapabilitySidsFromName")
	procSetEntriesInAclW              = winACAdvapi.NewProc("SetEntriesInAclW")
)

const (
	procThreadAttributeSecurityCapabilities = 0x00020009
	startfUseStdHandles                     = 0x00000100
	extendedStartupinfoPresent              = 0x00080000
	// EXPLICIT_ACCESS 形态
	grantAccess  = 1
	revokeAccess = 3
	// Trustee
	trusteeIsSid = 0
	// 继承：SUB_CONTAINERS_AND_OBJECTS_INHERIT=(OI)(CI)
	subContainersAndObjectsInherit = 0x3
	// Job
	jobObjectExtendedLimitInformation = 9
)

type winACSecurityCapabilities struct {
	appContainerSid *windows.SID
	capabilities    uintptr // *windows.SIDAndAttributes
	capabilityCount uint32
	reserved        uint32
}

// explicitAccessW EXPLICIT_ACCESS_W（amd64 布局 48B）。
type explicitAccessW struct {
	accessPermissions uint32
	accessMode        int32
	inheritance       uint32
	trustee           trusteeW
}
type trusteeW struct {
	multipleAccount       *trusteeW
	multipleAccountOp     int32
	trusteeForm           int32
	trusteeType           int32
	ptstrName             *uint16 // TrusteeForm=TRUSTEE_IS_SID 时=SID 指针
}

// ─── profile/SID 原语 ───

// winACCreateProfileFresh 创建 AppContainer profile——熵化命名+撞名有界重试
//（R-1667 v2——会议 #269 Jobs 裁决）：
//   - 名=GoalOS-AC-<action 净化前缀≤24>-<crypto/rand 64bit 熵 16hex>——profile 名
//     是一次性资源（同名重建永久损坏实机纪律），熵后缀构造性防撞；
//   - ALREADY_EXISTS（0x800700B7）=偶发撞名→换新名重试（上限 3 次）——
//     严禁 derive 复用（继承前序残留注册表/包虚拟化脏状态）；
//   - 其余任何失败=立即 fail-closed 带真实 HRESULT。
// 历史教训（2026-09-06 实机实锤）：pszDescription 不可为 NULL——=0 则 create
// 静默 E_INVALIDARG；若再叠「全失败 derive 兜底」=产出幻影 profile（derive 纯哈希
// 恒成功不要求实体存在），下游 spawn 报 file-not-found 且归因被误导。
func winACCreateProfileFresh(actionID string) (string, *windows.SID, error) {
	for attempt := 0; attempt < 3; attempt++ {
		var entropy [8]byte // 64bit——session 粒度防撞充裕
		if _, err := rand.Read(entropy[:]); err != nil {
			return "", nil, fmt.Errorf("熵源失败: %w", err)
		}
		name := fmt.Sprintf("GoalOS-AC-%s-%s", winACSanitizeN(actionID, 24), hex.EncodeToString(entropy[:]))
		namePtr, err := windows.UTF16PtrFromString(name)
		if err != nil {
			return "", nil, fmt.Errorf("profile 名含 NUL: %w", err)
		}
		var sid *windows.SID
		r1, _, callErr := procCreateAppContainerProfile.Call(
			uintptr(unsafe.Pointer(namePtr)), uintptr(unsafe.Pointer(namePtr)),
			uintptr(unsafe.Pointer(namePtr)), 0, 0, uintptr(unsafe.Pointer(&sid)))
		if r1 == 0 {
			return name, sid, nil
		}
		if uint32(r1) == 0x800700B7 { // HRESULT_FROM_WIN32(ERROR_ALREADY_EXISTS)=偶发撞名
			continue
		}
		return "", nil, fmt.Errorf("CreateAppContainerProfile %s: HRESULT=0x%08X: %v（fail-closed——R-1667 v2）", name, uint32(r1), callErr)
	}
	return "", nil, fmt.Errorf("profile 撞名重试 3 次耗尽（fail-closed——R-1667 v2）")
}

// winACDeleteProfile 删除（退避重试——R-1661 v2②；幂等=不存在不炸）。
func winACDeleteProfile(name string) error {
	namePtr, perr := windows.UTF16PtrFromString(name)
	if perr != nil {
		return fmt.Errorf("profile 名含 NUL: %w", perr)
	}
	var last error
	for i := 0; i < 10; i++ {
		r1, _, err := procDeleteAppContainerProfile.Call(uintptr(unsafe.Pointer(namePtr)))
		if r1 == 0 {
			return nil
		}
		last = err
		time.Sleep(200 * time.Millisecond)
	}
	// HRESULT_FROM_WIN32(ERROR_FILE_NOT_FOUND)=0x80070002=幂等通过（profile 不存在）
	if errno, ok := last.(syscall.Errno); ok && uint32(errno)&0xFFFF == 0x7002 {
		return nil
	}
	if last != nil && uint32(errnoOfErr(last))&0xFFFF == 0x0002 {
		return nil
	}
	return fmt.Errorf("DeleteAppContainerProfile %s: %v", name, last)
}

func errnoOfErr(err error) uintptr {
	if errno, ok := err.(syscall.Errno); ok {
		return uintptr(errno)
	}
	return 0
}

// winACDeriveCapabilitySID 具名能力 SID（S-1-15-3-* 族——纯名称派生零残留）。
func winACDeriveCapabilitySID(capName string) (*windows.SID, error) {
	namePtr, perr := windows.UTF16PtrFromString(capName)
	if perr != nil {
		return nil, fmt.Errorf("能力名含 NUL: %w", perr)
	}
	var groupSids, capSids *windows.SID
	var groupCount, capCount uint32
	r1, _, err := procDeriveCapabilitySidsFromName.Call(
		uintptr(unsafe.Pointer(namePtr)),
		uintptr(unsafe.Pointer(&groupSids)), uintptr(unsafe.Pointer(&groupCount)),
		uintptr(unsafe.Pointer(&capSids)), uintptr(unsafe.Pointer(&capCount)))
	if r1 == 0 {
		return nil, fmt.Errorf("DeriveCapabilitySidsFromName %s: %v", capName, err)
	}
	if capCount == 0 || capSids == nil {
		return nil, fmt.Errorf("能力 SID 数组为空: %s", capName)
	}
	defer windows.LocalFree(windows.Handle(unsafe.Pointer(groupSids)))
	first := *(**windows.SID)(unsafe.Pointer(capSids))
	copied, copyErr := first.Copy()
	windows.LocalFree(windows.Handle(unsafe.Pointer(capSids)))
	if copyErr != nil {
		return nil, copyErr
	}
	return copied, nil
}

// ─── ACL 授予/回收（SetEntriesInAclW 原生——R-1666 纪律：不借道 icacls 外壳） ───

// winACGrantACE 幂等授予（已授予=跳过——GetNamedSecurityInfo 查重）。
func winACGrantACE(path string, sid *windows.SID, perms uint32, inheritance uint32) error {
	present, err := winACACEPresent(path, sid)
	if err != nil {
		return err
	}
	if present {
		return nil
	}
	return winACSetEntries(path, sid, perms, inheritance, grantAccess)
}

// winACRevokeACE 回收（未授予=幂等通过）。
func winACRevokeACE(path string, sid *windows.SID) error {
	present, err := winACACEPresent(path, sid)
	if err != nil {
		return err
	}
	if !present {
		return nil
	}
	return winACSetEntries(path, sid, 0, 0, revokeAccess)
}

// winACACEPresent 查 ACE 是否已授予该 SID。
func winACACEPresent(path string, sid *windows.SID) (bool, error) {
	sd, err := windows.GetNamedSecurityInfo(path, windows.SE_FILE_OBJECT, windows.DACL_SECURITY_INFORMATION)
	if err != nil {
		return false, fmt.Errorf("GetNamedSecurityInfo %s: %w", path, err)
	}
	dacl, _, err := sd.DACL()
	if err != nil {
		return false, fmt.Errorf("DACL %s: %w", path, err)
	}
	if dacl == nil {
		return false, nil
	}
	for i := uint32(0); i < uint32(dacl.AceCount); i++ {
		var ace *windows.ACCESS_ALLOWED_ACE
		if err := windows.GetAce(dacl, i, &ace); err != nil {
			return false, err
		}
		aceSid := (*windows.SID)(unsafe.Pointer(&ace.SidStart))
		if windows.EqualSid(aceSid, sid) {
			return true, nil
		}
	}
	return false, nil
}

// winACSetEntries SetEntriesInAclW 应用（grant/revoke）+SetNamedSecurityInfoW 提交。
func winACSetEntries(path string, sid *windows.SID, perms uint32, inheritance uint32, mode int32) error {
	sd, err := windows.GetNamedSecurityInfo(path, windows.SE_FILE_OBJECT, windows.DACL_SECURITY_INFORMATION)
	if err != nil {
		return fmt.Errorf("GetNamedSecurityInfo %s: %w", path, err)
	}
	oldDacl, _, err := sd.DACL()
	if err != nil {
		return fmt.Errorf("DACL %s: %w", path, err)
	}
	ea := explicitAccessW{
		accessPermissions: perms,
		accessMode:        mode,
		inheritance:       inheritance,
		trustee: trusteeW{
			trusteeForm: trusteeIsSid,
			ptstrName:   (*uint16)(unsafe.Pointer(sid)),
		},
	}
	var newDacl *windows.ACL
	r1, _, err := procSetEntriesInAclW.Call(
		1, uintptr(unsafe.Pointer(&ea)),
		uintptr(unsafe.Pointer(oldDacl)),
		uintptr(unsafe.Pointer(&newDacl)))
	if r1 != 0 {
		return fmt.Errorf("SetEntriesInAclW %s: %v", path, err)
	}
	defer windows.LocalFree(windows.Handle(unsafe.Pointer(newDacl)))
	if err := windows.SetNamedSecurityInfo(path, windows.SE_FILE_OBJECT, windows.DACL_SECURITY_INFORMATION,
		nil, nil, newDacl, nil); err != nil {
		return fmt.Errorf("SetNamedSecurityInfo %s: %w", path, err)
	}
	return nil
}

// ─── Provider 主体 ───

// winACProvider Windows AppContainer 受限档 Provider。
type winACProvider struct {
	workspace  string
	tmpDir     string
	toolchains map[string]string // 具名工具链（name→安装根——R-1659 v3）
	capSids    map[string]*windows.SID
	prepared   bool
	mu         sync.Mutex
	// FD3（R-1650 v2/R-1660 v2——沙箱内回环转发器承接宿主服务）：dialFn=zone dialer
	// 注入（nil=broker 直连兜底——生产接线必须注入，runtime_wiring_windows.go）；
	// onDeny=broker 拒绝审计回调（nil=log 面）。
	dialFn fd3.DialFunc
	onDeny func(endpoint, reason string)
}

// WinACOption 构造可选项。
type WinACOption func(*winACProvider)

// WithDialFunc 注入 broker 拨号面（生产=zone dialer——网域分类留痕同源不旁路）。
func WithDialFunc(d fd3.DialFunc) WinACOption {
	return func(p *winACProvider) { p.dialFn = d }
}

// WithOnDeny 注入 broker 拒绝审计回调（默认=日志面）。
func WithOnDeny(fn func(endpoint, reason string)) WinACOption {
	return func(p *winACProvider) { p.onDeny = fn }
}

// NewWinACProvider 构造（toolchains=具名能力授权表 name→路径；nil=无工具链授予）。
func NewWinACProvider(workspace, tmpDir string, toolchains map[string]string, opts ...WinACOption) Provider {
	p := &winACProvider{
		workspace:  workspace,
		tmpDir:     tmpDir,
		toolchains: toolchains,
		capSids:    map[string]*windows.SID{},
	}
	for _, opt := range opts {
		opt(p)
	}
	return p
}

func (p *winACProvider) Name() string { return "winac-windows" }
func (p *winACProvider) Tier() string { return "T1" } // T1-WinAC（R-1652 v2 独立档——wire=T1 族）

// Prepare 一次性准备：WritableRoots 建目录+具名能力 SID 派生+工具链 RX 幂等授予。
func (p *winACProvider) Prepare(_ context.Context, _ RuntimePlan) error {
	p.mu.Lock()
	defer p.mu.Unlock()
	for _, root := range []string{p.workspace, p.tmpDir} {
		if err := os.MkdirAll(root, 0o755); err != nil {
			return fmt.Errorf("%w: WritableRoot 创建失败 %s: %w", ErrNoBackend, root, err)
		}
	}
	// 具名能力 SID 派生+工具链目录 RX 一次授予（R-1659 v3——幂等长期有效零残留）
	for name, path := range p.toolchains {
		capSid, err := winACDeriveCapabilitySID("GoalOS-TC-" + name)
		if err != nil {
			return fmt.Errorf("%w: 能力 SID 派生失败 %s: %w", ErrNoBackend, name, err)
		}
		p.capSids[name] = capSid
		if _, err := os.Stat(path); err != nil {
			return fmt.Errorf("%w: 工具链路径不存在 %s（%s）: %w", ErrNoBackend, name, path, err)
		}
		const genericReadExecute = windows.GENERIC_READ | windows.GENERIC_EXECUTE
		if err := winACGrantACE(path, capSid, genericReadExecute, subContainersAndObjectsInherit); err != nil {
			return fmt.Errorf("%w: 工具链授予失败 %s→%s: %w（fail-closed 禁止降级裸跑——R-1659③）", ErrNoBackend, name, path, err)
		}
	}
	p.prepared = true
	return nil
}

func (p *winACProvider) State(context.Context) (ProviderState, error) {
	if p.prepared {
		return ProviderPrepared, nil
	}
	return ProviderRegistered, nil
}

// Capabilities 能力快照（R-1652 v2 谓词分列——HasSyscallConfine 不自称）。
func (p *winACProvider) Capabilities(context.Context) (ProviderCapability, error) {
	return ProviderCapability{
		Platform:          "windows-appcontainer",
		AchievedIsolation: I2, // 读写双禁闭+网络禁闭+进程硬化+win32k 子集过滤；无 seccomp 等价物（不自称 I3）
		WarmPool:          false,
	}, nil
}

// Acquire 租约：session profile+Job（KILL_ON_JOB_CLOSE）+workspace/tmpDir 写面授予。
func (p *winACProvider) Acquire(_ context.Context, req LeaseRequest) (RuntimeHandle, error) {
	if !p.prepared {
		return nil, fmt.Errorf("%w: Prepare 未调用", ErrNoBackend)
	}
	// profile=熵化一次性名（R-1667 v2——撞名换新有界重试，derive 复用兜底已全删）
	profile, sid, err := winACCreateProfileFresh(req.ActionID)
	if err != nil {
		return nil, fmt.Errorf("runtime: AppContainer profile 创建失败: %w", err)
	}
	// Job（KILL_ON_JOB_CLOSE——内核级绞杀，R-1661 v2①）
	job, err := windows.CreateJobObject(nil, nil)
	if err != nil {
		windows.FreeSid(sid)
		return nil, fmt.Errorf("runtime: CreateJobObject: %w", err)
	}
	info := windows.JOBOBJECT_EXTENDED_LIMIT_INFORMATION{}
	info.BasicLimitInformation.LimitFlags = windows.JOB_OBJECT_LIMIT_KILL_ON_JOB_CLOSE
	if _, err := windows.SetInformationJobObject(job, jobObjectExtendedLimitInformation,
		uintptr(unsafe.Pointer(&info)), uint32(unsafe.Sizeof(info))); err != nil {
		windows.CloseHandle(job)
		windows.FreeSid(sid)
		return nil, fmt.Errorf("runtime: SetInformationJobObject: %w", err)
	}
	// 写面授予（session 包 SID 粒度——R-1659 v3④）：workspace+tmpDir=RWX+DELETE
	const rwMask = windows.GENERIC_READ | windows.GENERIC_WRITE | windows.GENERIC_EXECUTE | windows.DELETE
	for _, root := range []string{p.workspace, p.tmpDir} {
		if err := winACGrantACE(root, sid, rwMask, subContainersAndObjectsInherit); err != nil {
			windows.CloseHandle(job)
			windows.FreeSid(sid)
			_ = winACDeleteProfile(profile)
			return nil, fmt.Errorf("runtime: 写面授予失败 %s: %w（fail-closed）", root, err)
		}
	}
	return &winACHandle{
		p: p, profile: profile, sid: sid, job: job,
		state: HandleAcquired, goalID: req.GoalID,
		caps: winACSelectCaps(p.capSids, req.Contract),
		// R-1650 v2：契约声明端点集（FD3 broker 白名单——nil=无网络中继面）
		endpoints: ContractEndpoints(req.Contract),
	}, nil
}

// winACSelectCaps 按契约声明挑选具名能力（R-1659 v3②——"toolchain:<name>" 前缀映射；
// SE_GROUP_ENABLED=4——Attributes=0 静默无效实机实锤）。
func winACSelectCaps(capSids map[string]*windows.SID, contract *VerifiedContract) []windows.SIDAndAttributes {
	var caps []windows.SIDAndAttributes
	if contract == nil {
		return caps
	}
	for _, cap := range contract.Claims().Capabilities {
		if name, ok := strings.CutPrefix(cap, "toolchain:"); ok {
			if sid, found := capSids[name]; found {
				caps = append(caps, windows.SIDAndAttributes{Sid: sid, Attributes: 4})
			}
		}
	}
	return caps
}

// winACSanitizeN profile 段净化（AppContainer 名=字母数字有限字符集）+长度封顶。
func winACSanitizeN(s string, maxLen int) string {
	var b strings.Builder
	for _, r := range s {
		if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') || r == '-' || r == '_' {
			b.WriteRune(r)
		}
	}
	out := b.String()
	if len(out) > maxLen {
		out = out[:maxLen]
	}
	if out == "" {
		out = "x"
	}
	return out
}

// winACHandle 执行句柄。
type winACHandle struct {
	p       *winACProvider
	profile string
	sid     *windows.SID
	job     windows.Handle
	state   HandleState
	goalID  string
	caps    []windows.SIDAndAttributes
	mu      sync.Mutex
	// wg 在飞 Execute 计数——Release 绞杀依赖：state 翻转与绞杀在锁外进行时，
	// 须等在飞 execInContainer 收尾后才能 FreeSid/回收 ACE（防 use-after-free）。
	// 2026-09-06 实机实锤：Execute 全程持锁时 Release 锁死（sleeper 睡满全程），
	// 故锁只护 state，执行体与绞杀全在锁外。
	wg sync.WaitGroup
	// tainted=不可杀进程实锤标记（R-1669——会议 #269 Jobs 裁决）：execInContainer
	// 超时路径 TerminateProcess+2s bracket 仍未物理死亡=进程卡入不可中断内核态——
	// 此时严禁 FreeSid/DeleteProfile（宁承受单次句柄泄露，绝不在存活进程下释放
	// SID=UAF 焊死）。atomic：exec 锁外路径写，Release wg.Wait 后读（happens-after）。
	tainted atomic.Bool
	// FD3 面（R-1650 v2/R-1660 v2——沙箱内回环转发器承接宿主服务）：
	// endpoints=契约声明端点集；fd3Ln/broker=daemon 侧；fd3dUp=沙箱内转发器已拉起。
	endpoints []string
	fd3Ln     *fd3.Listener
	fd3Broker *fd3.Broker
	fd3dUp    bool
}

func (h *winACHandle) ID() string { return "winac-" + h.profile }

func (h *winACHandle) Start(ctx context.Context) error {
	h.mu.Lock()
	if h.state != HandleAcquired {
		h.mu.Unlock()
		return ErrInvalidState
	}
	// FD3 面（R-1650 v2）：契约声明端点集非空=拉中继（监听+broker+沙箱内转发器）；
	// 拉起失败=fail-closed（Start 失败=session 不开——不放行无中继的声明契约）。
	if len(h.endpoints) > 0 {
		if err := h.bringUpFD3(); err != nil {
			h.mu.Unlock()
			return fmt.Errorf("runtime: FD3 中继拉起失败（fail-closed）: %w", err)
		}
	}
	h.state = HandleReady
	h.mu.Unlock()
	return nil
}

// bringUpFD3 FD3 中继拉起：daemon 侧监听+broker（契约白名单+注入拨号）→
// 沙箱内 fd3d 拉起（detached——session 长驻，Job 绞杀收尾）→就绪证据门槛。
func (h *winACHandle) bringUpFD3() error {
	ln, err := fd3.Listen(h.p.tmpDir, "GoalOS-"+h.goalID)
	if err != nil {
		return err
	}
	broker := fd3.NewBroker(h.endpoints, h.p.dialFn, h.p.onDeny)
	go broker.Serve(ln)
	// 映射=透明同端口（agent 直连 localhost:<服务端已知端口> 无感知——R-1660 v2）
	mappings := strings.Join(h.endpoints, ",")
	outFile, err := h.spawnDetachedInContainer(probeSelfExe(),
		[]string{"__goalos-fd3d", ln.Name(), mappings})
	if err != nil {
		ln.Close()
		return err
	}
	// 就绪门槛：fd3d 证据文件出现 FD3D-READY（跨 loopback 不可达——捕获文件=唯一证据面）
	var mapLines []string
	deadline := time.Now().Add(8 * time.Second)
	for time.Now().Before(deadline) {
		if data, rerr := os.ReadFile(outFile); rerr == nil && strings.Contains(string(data), "FD3D-READY") {
			for _, line := range strings.Split(string(data), "\n") {
				if strings.HasPrefix(line, "FD3D-MAP ") {
					mapLines = append(mapLines, strings.TrimPrefix(line, "FD3D-MAP "))
				}
			}
			break
		}
		time.Sleep(100 * time.Millisecond)
	}
	if mapLines == nil {
		ln.Close()
		data, _ := os.ReadFile(outFile)
		return fmt.Errorf("fd3d 就绪证据超时（捕获文件=%s 无 FD3D-READY——内容=%q）", outFile, string(data))
	}
	// 实际映射落工作区（绑定冲突回落后「声明≠实际」必须显式可见——禁静默错配；
	// 工作负载读 goalos-fd3-map.txt 知真实回环端口——每行 <listenPort>=<endpoint>）
	mapContent := strings.Join(mapLines, "\n") + "\n"
	if err := os.WriteFile(filepath.Join(h.p.workspace, "goalos-fd3-map.txt"), []byte(mapContent), 0644); err != nil {
		ln.Close()
		return fmt.Errorf("fd3 映射文件写入失败: %w", err)
	}
	h.fd3Ln, h.fd3Broker, h.fd3dUp = ln, broker, true
	return nil
}

// Precheck 边界实证（原生探针经 AC 执行——R-1666 ERRNO 数字断言；fail-closed）。
func (h *winACHandle) Precheck(ctx context.Context) error {
	h.mu.Lock()
	if h.state != HandleReady && h.state != HandleRunning {
		h.mu.Unlock()
		return ErrInvalidState
	}
	h.wg.Add(1)
	h.mu.Unlock()
	defer h.wg.Done()
	home, _ := os.UserHomeDir()
	// ①fs 探针：写 home=必拒（受限档 DenyWrite=home 契约面）
	probePath := filepath.Join(home, "goalos-winac-precheck.txt")
	code, out := h.execInContainer(ctx, probeSelfExe(), []string{"__goalos-probe", "write", probePath})
	_ = os.Remove(probePath)
	if code == 0 || !strings.Contains(out, "PROBE-ERRNO=") || strings.Contains(out, "PROBE-ERRNO=0") {
		return fmt.Errorf("runtime: Precheck fs 探针未被拒（写 home 成功=AC 边界失效）——out=%q", out)
	}
	// ②网络探针：出站必拒（零 capability——ERRNO=WSAEACCES 族）
	code, out = h.execInContainer(ctx, probeSelfExe(), []string{"__goalos-probe", "dial", "192.0.2.1:80"})
	if code == 0 || strings.Contains(out, "PROBE-ERRNO=0") {
		return fmt.Errorf("runtime: Precheck 网络探针未被拒（出站成功=零 capability 失效=边界失效）——out=%q", out)
	}
	h.mu.Lock()
	if h.state == HandleReleased || h.state == HandleDestroyed {
		h.mu.Unlock()
		return ErrInvalidState // 探针期间被 Release——不翻回 Running
	}
	h.state = HandleRunning
	h.mu.Unlock()
	return nil
}

// probeSelfExe 探针载体=自身二进制（R-1666 零外部运行时）。
func probeSelfExe() string {
	self, err := os.Executable()
	if err != nil {
		return `C:\Windows\System32\cmd.exe` // 必败方向（exec 失败=非零）
	}
	return self
}

// Execute 边界内执行（process.exec 唯一动词——CWD=workspace 显式设定 F6）。
func (h *winACHandle) Execute(ctx context.Context, req ExecuteRequest) (ExecuteResult, error) {
	h.mu.Lock()
	if h.state != HandleRunning {
		h.mu.Unlock()
		return ExecuteResult{}, ErrInvalidState
	}
	h.wg.Add(1) // 锁内登记——Release 的 state 翻转与 Add 互斥，计数无漏
	h.mu.Unlock()
	defer h.wg.Done()
	if req.ActionType != "process.exec" {
		return ExecuteResult{}, fmt.Errorf("runtime: 受限档不支持的能力动词 %q（process.exec 唯一）", req.ActionType)
	}
	binary := req.Params["binary"]
	if binary == "" {
		return ExecuteResult{}, fmt.Errorf("runtime: process.exec 缺 binary 参数（代理层拒绝——未触 OS 边界）")
	}
	start := time.Now()
	code, out := h.execInContainer(ctx, binary, strings.Fields(req.Params["args"]))
	res := ExecuteResult{Output: out, ExitCode: code, Status: "success", Cost: time.Since(start)}
	if code != 0 {
		res.Status = "failed"
	}
	return res, nil
}

// spawnInContainer AC 内拉起子进程（SUSPENDED 创建→入 Job→Resume——消灭先跑后入
// job 的竞态窗口；输出=tmpDir 捕获文件继承句柄）。返回=捕获文件路径+进程信息
//（句柄所有权移交调用方）。execInContainer=同步等待面；spawnDetachedInContainer=
// 长驻面（fd3d 族——不等待不读回，Job KILL_ON_JOB_CLOSE 收尾）。
func (h *winACHandle) spawnInContainer(binary string, args []string) (string, windows.ProcessInformation, error) {
	var pi windows.ProcessInformation
	outFile := filepath.Join(h.p.tmpDir, fmt.Sprintf("winac-out-%d.txt", time.Now().UnixNano()))
	outPtr, perr := windows.UTF16PtrFromString(outFile)
	if perr != nil {
		return "", pi, fmt.Errorf("捕获文件路径含 NUL")
	}
	sa := &windows.SecurityAttributes{Length: uint32(unsafe.Sizeof(windows.SecurityAttributes{})), InheritHandle: 1}
	hOut, err := windows.CreateFile(outPtr, windows.GENERIC_WRITE, windows.FILE_SHARE_READ, sa,
		windows.CREATE_ALWAYS, windows.FILE_ATTRIBUTE_NORMAL, 0)
	if err != nil {
		return "", pi, fmt.Errorf("捕获文件: %w", err)
	}
	defer windows.CloseHandle(hOut)

	secCaps := winACSecurityCapabilities{appContainerSid: h.sid}
	if len(h.caps) > 0 {
		secCaps.capabilities = uintptr(unsafe.Pointer(&h.caps[0]))
		secCaps.capabilityCount = uint32(len(h.caps))
	}
	attrList, err := windows.NewProcThreadAttributeList(1)
	if err != nil {
		return "", pi, fmt.Errorf("attrlist: %w", err)
	}
	defer attrList.Delete()
	if err := attrList.Update(procThreadAttributeSecurityCapabilities, unsafe.Pointer(&secCaps), unsafe.Sizeof(secCaps)); err != nil {
		return "", pi, fmt.Errorf("attr update: %w", err)
	}

	var siex windows.StartupInfoEx
	siex.Cb = uint32(unsafe.Sizeof(siex))
	siex.Flags = startfUseStdHandles
	siex.StdOutput = hOut
	siex.StdErr = hOut
	siex.ProcThreadAttributeList = attrList.List()

	// 命令行组装（引号包裹防空格断词）
	cmdline := `"` + binary + `"`
	for _, a := range args {
		cmdline += ` "` + a + `"`
	}
	cmdPtr, perr := windows.UTF16PtrFromString(cmdline)
	if perr != nil {
		return "", pi, fmt.Errorf("命令行含 NUL")
	}
	cwdPtr, perr := windows.UTF16PtrFromString(h.p.workspace) // F6：CWD 显式=workspace（已授予面）
	if perr != nil {
		return "", pi, fmt.Errorf("workspace 路径含 NUL")
	}

	err = windows.CreateProcess(nil, cmdPtr, nil, nil, true,
		windows.CREATE_SUSPENDED|extendedStartupinfoPresent|windows.CREATE_UNICODE_ENVIRONMENT,
		nil, cwdPtr, &siex.StartupInfo, &pi)
	if err != nil {
		return "", pi, fmt.Errorf("CreateProcess: %w cmdline=%s cwd=%s", err, cmdline, h.p.workspace)
	}
	// 入 Job 先于 Resume（KILL_ON_JOB_CLOSE 覆盖全生命周期——无竞态窗口）
	if err := windows.AssignProcessToJobObject(h.job, pi.Process); err != nil {
		windows.TerminateProcess(pi.Process, 1)
		windows.CloseHandle(pi.Process)
		windows.CloseHandle(pi.Thread)
		return "", pi, fmt.Errorf("AssignProcessToJobObject: %w", err)
	}
	if _, err := windows.ResumeThread(pi.Thread); err != nil {
		windows.CloseHandle(pi.Process)
		windows.CloseHandle(pi.Thread)
		return "", pi, fmt.Errorf("ResumeThread: %w", err)
	}
	return outFile, pi, nil
}

// spawnDetachedInContainer 长驻子进程拉起（fd3d 族——不等不收；句柄即关
//（Job 成员身份已建立=KILL_ON_JOB_CLOSE 覆盖，进程句柄非存活依据））。
func (h *winACHandle) spawnDetachedInContainer(binary string, args []string) (string, error) {
	outFile, pi, err := h.spawnInContainer(binary, args)
	if err != nil {
		return "", err
	}
	windows.CloseHandle(pi.Process)
	windows.CloseHandle(pi.Thread)
	return outFile, nil
}

// execInContainer AppContainer 内执行（同步面——spawn→等待→读回捕获文件）。
func (h *winACHandle) execInContainer(_ context.Context, binary string, args []string) (int, string) {
	outFile, pi, err := h.spawnInContainer(binary, args)
	defer os.Remove(outFile)
	if err != nil {
		return -1, "WINAC-FATAL: " + err.Error()
	}
	defer windows.CloseHandle(pi.Process)
	defer windows.CloseHandle(pi.Thread)
	wait, err := windows.WaitForSingleObject(pi.Process, 120000)
	if err != nil || wait != 0 {
		_ = windows.TerminateProcess(pi.Process, 1)
		// R-1669 物理死亡 bracket：TerminateProcess 异步——2s 内未死=进程卡入
		// 不可中断内核/驱动态，标记 tainted（Release 跳过 FreeSid/DeleteProfile——
		// 宁泄露句柄绝不在存活进程下释放 SID，UAF 焊死）。
		ev, werr := windows.WaitForSingleObject(pi.Process, 2000)
		if werr != nil || ev != windows.WAIT_OBJECT_0 {
			h.tainted.Store(true)
			return -1, fmt.Sprintf("WINAC-FATAL: 等待超时且强杀 2s bracket 未确认物理死亡（ev=%d werr=%v——内核态 wedge，句柄已 tainted，Release 泄露式跳过回收）", ev, werr)
		}
		return -1, "WINAC-FATAL: 等待失败/超时（已强杀并确认物理死亡）"
	}
	var code uint32
	_ = windows.GetExitCodeProcess(pi.Process, &code)
	// 捕获文件读回（hOut 句柄归 spawn 方关闭；子进程已退出=写入已完结——
	// Flush 面随句柄所有权退役，2026-09-07 spawn 拆分注记）。
	data, _ := os.ReadFile(outFile)
	return int(code), string(data)
}

// Release 清理（R-1661 v2）：翻 state→关 Job（绞杀在飞+全部子孙，锁外——
// 锁内关=与在飞 Execute 死锁实锤）→等在飞收尾→收 ACE→删 profile（退避重试）
// →幂等（二次调用=成功）。
func (h *winACHandle) Release(context.Context) error {
	h.mu.Lock()
	if h.state == HandleReleased || h.state == HandleDestroyed {
		h.mu.Unlock()
		return nil // 幂等
	}
	h.state = HandleReleased
	h.mu.Unlock()
	if h.fd3Ln != nil {
		h.fd3Ln.Close() // broker Accept 循环退出（fd3d 由 Job 绞杀——下同族覆盖）
	}
	windows.CloseHandle(h.job) // KILL_ON_JOB_CLOSE=内核级绞杀（先绞杀再等收尾——反序=白等在飞超时）
	h.wg.Wait()                // 在飞 execInContainer 收尾后方可 FreeSid/收 ACE
	var errs []string
	// ACE 回收不受 taint 影响（收窄存活进程访问面=安全方向；不触碰 SID 内存/profile 实体）
	for _, root := range []string{h.p.workspace, h.p.tmpDir} {
		if err := winACRevokeACE(root, h.sid); err != nil {
			errs = append(errs, err.Error())
		}
	}
	if h.tainted.Load() {
		// R-1669 fail-safe：不可杀进程仍在存活——FreeSid/DeleteProfile 焊死跳过
		//（宁泄露单次句柄+profile 残留，绝不在存活进程下释放=UAF 构造性不可能）。
		// 泄露面=注册表 Mappings 项+Packages 虚拟化目录（profile 名熵化一次性，零复用零污染后续 session）。
		// state 已于本函数顶部锁内翻转=HandleReleased（幂等不受损）。
		return fmt.Errorf("runtime: CRITICAL 句柄 tainted（不可杀进程存活）——已泄露式跳过 profile/SID 回收（R-1669 fail-safe；泄露 profile=%s 需人工核查）",
			h.profile)
	}
	if err := winACDeleteProfile(h.profile); err != nil {
		errs = append(errs, err.Error())
	}
	windows.FreeSid(h.sid)
	if len(errs) > 0 {
		return fmt.Errorf("runtime: Release 部分失败: %s", strings.Join(errs, "; "))
	}
	return nil
}

// Interrupt/Pause/Resume——无长驻子进程面（Execute 同步生命周期），契约表语义。
func (h *winACHandle) Interrupt(context.Context) error {
	if h.state != HandleRunning {
		return ErrInvalidState
	}
	return nil
}

func (h *winACHandle) Pause(context.Context) error {
	if h.state != HandleRunning {
		return ErrInvalidState
	}
	h.state = HandlePaused
	return nil
}

func (h *winACHandle) Resume(context.Context) error {
	if h.state != HandlePaused {
		return ErrInvalidState
	}
	h.state = HandleRunning
	return nil
}

// State 句柄状态（全态可调——契约表）。
func (h *winACHandle) State() HandleState { return h.state }
