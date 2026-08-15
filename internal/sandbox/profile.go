package sandbox

import (
	"fmt"
)

// profile.go — TOML Profile 解析与校验（任务 1.2 骨架）。
//
// 契约（08 §11）：TOML Profile=声明式安全契约文件——文件系统白名单/网络白名单/exec 分层策略/
// 资源限制/环境变量注入。解析器=BurntSushi/toml（R-907）；缓存键=内容 SHA-256（R-908，废 mtime）；
// 三层合并语义=deny 取并集、allow 取交集（R-908）。安全约束不弱于 daemon.yaml 全局策略。
//
// 零值非法化（R-1106）：CompiledProfile 必须经 Compile() 产出——直接构造的零值在 Execute 入口
// 被 Fatal fail-closed 拒绝（compiled 标记=false）。

// RawProfile — TOML 原始结构（解析后未校验）。
type RawProfile struct {
	Isolation  string            `toml:"isolation"`  // I0-I5 请求等级（编译期降级为实际生效，R-1012）
	Filesystem FilesystemSection `toml:"filesystem"` // 文件系统白名单
	Network    NetworkSection    `toml:"network"`    // 网络白名单
	Resources  ResourcesSection  `toml:"resources"`  // 资源限制（R-1155 四键）
}

// FilesystemSection — [filesystem] 节。
type FilesystemSection struct {
	AllowRead  []string `toml:"allow_read"`
	AllowWrite []string `toml:"allow_write"`
	Deny       []string `toml:"deny"`
}

// NetworkSection — [network] 节（R-1156 网络能力插件 socket 全拒+出网代理白名单）。
type NetworkSection struct {
	Mode      string   `toml:"mode"`      // "deny"|"allowlist"
	Allowlist []string `toml:"allowlist"` // 出网代理白名单
}

// ResourcesSection — [resources] 节（R-1155 四键）。
type ResourcesSection struct {
	MaxMemoryMB  int `toml:"max_memory_mb"`
	MaxCPUCores  int `toml:"max_cpu_cores"`
	MaxDiskMB    int `toml:"max_disk_mb"`
	MaxProcesses int `toml:"max_processes"`
}

// Validate — Rulebook V1-V12（08 §11.5）。骨架：非法值拒绝，合法值通过。
// 校验失败=返回描述性 error（字段名+约束名）——不静默降级。
func (r *RawProfile) Validate() error {
	if r == nil {
		return fmt.Errorf("profile: 零值非法（R-1106）——必须经 Compile() 流程产出")
	}
	switch r.Isolation {
	case "I0", "I1", "I2", "I3", "I4", "I5":
		// 合法
	case "":
		return fmt.Errorf("profile.isolation: 缺失——必须显式声明 I0-I5（无隐式默认降级，R-1012）")
	default:
		return fmt.Errorf("profile.isolation: 非法值 %q——取值域=I0..I5", r.Isolation)
	}
	switch r.Network.Mode {
	case "deny", "allowlist", "":
		// 合法（空=默认 deny）
	default:
		return fmt.Errorf("profile.network.mode: 非法值 %q——取值域=deny|allowlist", r.Network.Mode)
	}
	if r.Resources.MaxMemoryMB < 0 || r.Resources.MaxCPUCores < 0 || r.Resources.MaxDiskMB < 0 || r.Resources.MaxProcesses < 0 {
		return fmt.Errorf("profile.resources: 负值非法（R-1155 四键非负）")
	}
	return nil
}

// Compile — TOML→CompiledProfile 编译（R-907 解析+R-908 内容 SHA-256 缓存键+R-1012 降级一次性确定）。
// 骨架：校验→构造 CompiledProfile（compiled=true 标记）。平台域编译规则归 backend_*。
func Compile(raw *RawProfile, platform PlatformID) (*CompiledProfile, error) {
	if err := raw.Validate(); err != nil {
		return nil, err
	}
	p := &CompiledProfile{
		platform:   platform,
		isolation:  parseIsolation(raw.Isolation),
		filesystem: CompiledFilesystem{AllowPaths: append(append([]string{}, raw.Filesystem.AllowRead...), raw.Filesystem.AllowWrite...)},
		network:    CompiledNetwork{Mode: raw.Network.Mode},
		process:    CompiledProcess{MaxProcesses: raw.Resources.MaxProcesses},
		extensions: map[string]any{},
		compiled:   true, // R-1106：Compile 产出标记
	}
	return p, nil
}

// parseIsolation — I0-I5 字符串→int（0-5）。
func parseIsolation(s string) int {
	if len(s) == 2 && s[0] == 'I' && s[1] >= '0' && s[1] <= '5' {
		return int(s[1] - '0')
	}
	return -1 // 非法（Validate 已拦截，此为防御性回退）
}
