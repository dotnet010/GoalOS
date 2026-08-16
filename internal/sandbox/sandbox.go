// Package sandbox 是 GoalOS v0.3.0 的架构中心制品（R-926）：沙箱抽象层。
//
// 职责边界（R-1111）：Sandbox=执行环境构造（namespace/profile 编译/运行时挂载集），
// 不管理插件进程生命周期（spawn/IPC/状态机归 PluginRunner）。
//
// 治理不变量（R-906a）：sandbox.Execute 的所有调用点必须位于 Governance 治理闸门之后——
// 调用链：PipelineRunner → 五引擎 → Plugin Runner → sandbox.Execute。本包不 import
// governance/pluginrunner（保持低层纯净），由调用方保证顺序。
//
// 类型权威：本文件类型定义以 05 §X.1a 为权威（R-1008 数据流）。零值非法化（R-1106）：
// CompiledProfile 零值=编译未完成，Execute 入口零值→Fatal fail-closed。
package sandbox

import (
	"context"
	"time"
)

// PlatformID 编译目标平台（05 §X.1a 权威）。
type PlatformID string

const (
	PlatformWindows   PlatformID = "windows"
	PlatformLinux     PlatformID = "linux"
	PlatformDarwin    PlatformID = "darwin"
	PlatformXinchuang PlatformID = "xinchuang"
)

// ResourceLimit 进程树资源上限（05 §X.1a 权威）。0/false=不设限制（由 profile [resources] 显式填充）。
type ResourceLimit struct {
	CPUTime     time.Duration // CPU 时间上限（0=不限）。Linux rlimit CPU / Windows Job PerProcessUserTimeLimit
	MemoryBytes uint64        // 内存上限（0=不限）。Linux rlimit AS / Windows Job 工作集
	OpenFiles   int           // 打开文件数上限（0=不限）。Linux rlimit NOFILE
	ProcessTree bool          // true=限制作用于整个进程树（R-957 含孙子）；false=仅直接子进程
}

// StdioMode 数据通道模式（05 §X.1a 权威）。
type StdioMode int

const (
	StdioInherit StdioMode = iota // 直接继承终端（Goal 模式默认）
	StdioCapture                  // 捕获到数据通道文件（Result.StdoutPath/StderrPath）
	StdioFile                     // 重定向到显式路径
)

// CommandSpec — Sandbox.Execute 的 cmd 参数（R-988）。不暴露 *exec.Cmd——Windows 不支持 ExtraFiles。
// 与 Plugin Runner 关系：ExecuteMessage（IPC 层）→ 翻译 → CommandSpec（Sandbox 层）。
type CommandSpec struct {
	Path       string        // 可执行文件绝对路径（解析符号链接，R-913）
	Args       []string      // 参数（不含 argv[0]）
	Env        []string      // 环境变量 "KEY=VALUE"（含 ${secret:xxx} 解密后注入——R-917 红线）
	WorkingDir string        // 工作目录（=profile workspace）
	Stdio      StdioMode     // INHERIT|CAPTURE|FILE（数据通道目录，R-951）
	Resources  ResourceLimit // 进程树资源上限
	Timeout    time.Duration // 进程树超时（max_execution_time 透传）
}

// CompiledProfile — Sandbox.Execute 的 profile 参数（R-1008）。
// 零值非法化（R-1106/R-1152）：Execute 入口零值→Fatal fail-closed——I0 也必须经 Compile() 显式构造。
type CompiledProfile struct {
	isolation  int                // 编译后实际生效隔离等级（I0-I5，R-1115 尺度归一）。Compile() 一次性确定，生命周期内不可变（R-1012）
	platform   PlatformID         // 编译目标平台
	syscall    CompiledSyscall    // 系统调用规则（seccomp BPF / SACL / Seatbelt，按平台）
	filesystem CompiledFilesystem // 文件系统规则（ACL / mount ns / 路径白名单）
	network    CompiledNetwork    // 网络规则
	process    CompiledProcess    // 进程规则（Job Object / PID ns / cgroup）
	extensions map[string]any     // 平台特定扩展——仅目标平台实现解释
	compiled   bool               // R-1106：Compile() 产出标记——false=零值非法（Execute 入口 Fatal fail-closed）
}

// 导出 getter（读路径；写路径仅 Compile() 内部——R-1106 零值非法化）。
func (p CompiledProfile) IsolationLevel() int            { return p.isolation }
func (p CompiledProfile) Platform() PlatformID           { return p.platform }
func (p CompiledProfile) Syscall() CompiledSyscall       { return p.syscall }
func (p CompiledProfile) Filesystem() CompiledFilesystem { return p.filesystem }
func (p CompiledProfile) Network() CompiledNetwork       { return p.network }
func (p CompiledProfile) Process() CompiledProcess       { return p.process }
func (p CompiledProfile) Extensions() map[string]any     { return p.extensions }
func (p CompiledProfile) Compiled() bool                 { return p.compiled }

// CompiledSyscall / CompiledFilesystem / CompiledNetwork / CompiledProcess — 平台域子结构（08 §19 枚举）。
type CompiledSyscall struct {
	BackendID string   // "linux-sandbox"|"xinchuang-sandbox"|"darwin-seatbelt"|"windows-job"（R-1399 命名）
	Rules     []string // 已编译规则摘要（实现细节归 backend）
}
type CompiledFilesystem struct{ AllowPaths []string }
type CompiledNetwork struct{ Mode string } // "deny"|"allowlist"
type CompiledProcess struct{ MaxProcesses int }

// Result — Sandbox.Execute 的统一返回（05 §X.1a 权威）。四后端必须返回相同结构。
type Result struct {
	ExitCode     int               // 子进程退出码透传。沙箱拒绝执行时=-1（配合 error 四态）
	Signal       int               // R-1370：被信号杀死时=信号号；正常退出=0
	StdoutPath   string            // stdout 大 payload 文件引用（R-951）
	StderrPath   string            // stderr 大 payload 文件引用
	StdoutTail   []byte            // stdout 尾部内联（≤4KB，快速诊断）
	StderrTail   []byte            // stderr 尾部内联（≤4KB）
	Duration     time.Duration     // 墙钟执行时间（子进程启动→退出）
	PIDTree      []int             // 进程树镜像（审计，R-957）；可为空但须在 BackendMeta 说明
	BackendID    string            // "windows-job"|"xinchuang-sandbox"|"linux-sandbox"|"darwin-seatbelt"（R-1399）
	PlatformTier int               // 实际生效平台支持级别（1/2/3，R-924；R-1115 更名自 SandboxLevel）
	BackendMeta  map[string]string // 后端特有元数据（Windows Job 名、namespace ID）
}

// Sandbox — 沙箱抽象接口（05 §X.1a 权威，R-1009/R-1010 生命周期/ctx 语义）。
// error 语义边界（R-973）：error 仅描述沙箱基础设施失败（Job Object 创建失败/seccomp 加载失败/
// IPC 超时/后端失效/fail-closed 拒绝——§X.1b 矩阵五分支）；子进程业务失败（exit≠0）走
// Result.ExitCode，error=nil。禁止"允许执行但返回 NeedHuman error"。
type Sandbox interface {
	Execute(ctx context.Context, cmd *CommandSpec, profile CompiledProfile) (*Result, error)
	Close() error
}

// PlatformCapabilities — detect.go 启动期探测返回（R-1003/R-947/R-976 生命周期，R-1189 追加）。
type PlatformCapabilities struct {
	KernelVersion   string // "4.19.0"/"5.15.0"（uname -r）
	LandlockABI     int    // 0=不可用（<5.13）；1-5=ABI 版本
	SeccompNotify   bool   // SECCOMP_IOCTL_NOTIF_RECV 可用（>=5.0）
	UserNamespace   bool   // unshare(CLONE_NEWUSER|CLONE_NEWNS) 实测可用
	JobObject       bool   // Windows CreateJobObjectW 可用
	MXCInstalled    bool   // wxc-exec 存在（v0.3.0 仅检测，R-958 stub）
	SimulatedKernel string // 模拟开关生效时的内核版本（""=未模拟）
	// 模拟开关（R-929/R-960）：GOALOS_SANDBOX_SIMULATE_KERNEL=4.19 整包模拟；
	// per-feature: GOALOS_SANDBOX_DISABLE_LANDLOCK/_SECCOMP_NOTIFY/_NAMESPACE/_ALL
}
